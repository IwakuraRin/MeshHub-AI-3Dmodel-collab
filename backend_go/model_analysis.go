/*
|--------------------------------------------------------------------------
| 本地三维模型分析服务
|--------------------------------------------------------------------------
| 负责识别本地模型格式，提取基础几何信息，并为前端预览提供文件内容。
|--------------------------------------------------------------------------
*/
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

/*
|--------------------------------------------------------------------------
| 模块能力清单
|--------------------------------------------------------------------------
| 文件选择：通过 Wails 打开本地三维模型文件。
| 格式识别：识别 STL、OBJ、FBX、STEP/STP、SolidWorks 官方格式。
| 几何分析：统计 STL 三角面、OBJ 顶点和面，并计算可用包围盒。
| 预览读取：读取可直接预览格式，返回给前端 Three.js 加载。
| 上传读取：读取支持格式的模型文件，交给前端上传云端。
|--------------------------------------------------------------------------
*/

/*
|--------------------------------------------------------------------------
| 模型分析错误
|--------------------------------------------------------------------------
| 定义可预期错误，让前端和调用者能看懂失败原因。
|--------------------------------------------------------------------------
*/
var (
	ErrEmptyModelPath        = errors.New("模型文件路径不能为空")
	ErrUnsupportedModelFile  = errors.New("不支持的三维模型文件格式")
	ErrPreviewNeedsConvert   = errors.New("该模型需要服务端转换后才能预览")
	ErrPreviewFileTooLarge   = errors.New("模型文件过大，暂不支持直接读取到前端预览")
	ErrModelAnalyzerNotReady = errors.New("模型分析服务尚未准备好")
)

/*
|--------------------------------------------------------------------------
| 分析服务配置
|--------------------------------------------------------------------------
| 控制支持格式、可直接预览格式和单次传给前端预览的文件大小上限。
|--------------------------------------------------------------------------
*/
const maxPreviewFileBytes = 200 * 1024 * 1024
const maxUploadFileBytes = 256 * 1024 * 1024

var supportedModelExtensions = map[string]ModelFormat{
	"stl":    {Name: "STL", Family: "mesh", Previewable: true, NeedsConversion: false, PreviewFormat: "stl"},
	"obj":    {Name: "OBJ", Family: "mesh", Previewable: true, NeedsConversion: false, PreviewFormat: "obj"},
	"fbx":    {Name: "FBX", Family: "scene", Previewable: true, NeedsConversion: false, PreviewFormat: "fbx"},
	"stp":    {Name: "STEP", Family: "cad", Previewable: false, NeedsConversion: true, PreviewFormat: "stl/obj"},
	"step":   {Name: "STEP", Family: "cad", Previewable: false, NeedsConversion: true, PreviewFormat: "stl/obj"},
	"sldprt": {Name: "SolidWorks Part", Family: "solidworks", Previewable: false, NeedsConversion: true, PreviewFormat: "stl/obj"},
	"sldasm": {Name: "SolidWorks Assembly", Family: "solidworks", Previewable: false, NeedsConversion: true, PreviewFormat: "stl/obj"},
	"slddrw": {Name: "SolidWorks Drawing", Family: "solidworks", Previewable: false, NeedsConversion: true, PreviewFormat: "stl/obj"},
}

/*
|--------------------------------------------------------------------------
| 模型分析服务
|--------------------------------------------------------------------------
| 提供本地文件选择、格式分析和预览文件读取能力。
|--------------------------------------------------------------------------
*/
type ModelAnalyzer struct{}

/*
|--------------------------------------------------------------------------
| 模型格式描述
|--------------------------------------------------------------------------
| 描述格式名称、格式族、能否直接预览，以及需要转换到什么预览格式。
|--------------------------------------------------------------------------
*/
type ModelFormat struct {
	Name            string
	Family          string
	Previewable     bool
	NeedsConversion bool
	PreviewFormat   string
}

/*
|--------------------------------------------------------------------------
| 模型分析结果
|--------------------------------------------------------------------------
| 返回给前端展示的本地模型文件信息和基础几何统计。
|--------------------------------------------------------------------------
*/
type ModelAnalysis struct {
	ID              string           `json:"id"`
	Path            string           `json:"path"`
	Name            string           `json:"name"`
	Extension       string           `json:"extension"`
	Size            int64            `json:"size"`
	SizeLabel       string           `json:"sizeLabel"`
	FormatName      string           `json:"formatName"`
	FormatFamily    string           `json:"formatFamily"`
	Previewable     bool             `json:"previewable"`
	NeedsConversion bool             `json:"needsConversion"`
	PreviewFormat   string           `json:"previewFormat"`
	Summary         string           `json:"summary"`
	Details         []AnalysisDetail `json:"details"`
	Bounds          *ModelBounds     `json:"bounds,omitempty"`
}

/*
|--------------------------------------------------------------------------
| 分析详情项
|--------------------------------------------------------------------------
| 用键值结构表达三角面、顶点数、实体数量等适合展示在前端的信息。
|--------------------------------------------------------------------------
*/
type AnalysisDetail struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

/*
|--------------------------------------------------------------------------
| 模型包围盒
|--------------------------------------------------------------------------
| 保存可以从 STL 或 OBJ 中计算出的模型三维范围。
|--------------------------------------------------------------------------
*/
type ModelBounds struct {
	MinX float64 `json:"minX"`
	MinY float64 `json:"minY"`
	MinZ float64 `json:"minZ"`
	MaxX float64 `json:"maxX"`
	MaxY float64 `json:"maxY"`
	MaxZ float64 `json:"maxZ"`
}

/*
|--------------------------------------------------------------------------
| 预览模型载荷
|--------------------------------------------------------------------------
| 把本地可预览模型读成 Base64，供前端创建 Blob 后交给 Three.js 加载。
|--------------------------------------------------------------------------
*/
type PreviewModelPayload struct {
	FileName  string `json:"fileName"`
	Extension string `json:"extension"`
	MimeType  string `json:"mimeType"`
	Base64    string `json:"base64"`
}

/*
|--------------------------------------------------------------------------
| 上传模型载荷
|--------------------------------------------------------------------------
| 把本地模型读成 Base64，供前端构造表单并上传云端，不写入本地缓存。
|--------------------------------------------------------------------------
*/
type UploadModelPayload struct {
	FileName  string `json:"fileName"`
	Extension string `json:"extension"`
	Size      int64  `json:"size"`
	MimeType  string `json:"mimeType"`
	Base64    string `json:"base64"`
}

/*
|--------------------------------------------------------------------------
| 模型分析服务创建
|--------------------------------------------------------------------------
| 创建本地模型分析服务实例，后续由 App 对外暴露给 Vue 前端调用。
|--------------------------------------------------------------------------
*/
func NewModelAnalyzer() *ModelAnalyzer {
	return &ModelAnalyzer{}
}

/*
|--------------------------------------------------------------------------
| 打开并分析本地模型文件
|--------------------------------------------------------------------------
| 通过 Wails 文件对话框选择模型文件，并逐个返回本地分析结果。
|--------------------------------------------------------------------------
*/
func (a *ModelAnalyzer) ImportModelFiles(ctx context.Context) ([]ModelAnalysis, error) {
	if ctx == nil {
		return nil, ErrModelAnalyzerNotReady
	}

	paths, err := runtime.OpenMultipleFilesDialog(ctx, runtime.OpenDialogOptions{
		Title: "导入三维模型文件",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "三维模型文件 (*.stp;*.step;*.stl;*.sldprt;*.sldasm;*.slddrw;*.obj;*.fbx)",
				Pattern:     "*.stp;*.step;*.stl;*.sldprt;*.sldasm;*.slddrw;*.obj;*.fbx",
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("打开本地模型文件失败: %w", err)
	}

	results := make([]ModelAnalysis, 0, len(paths))
	for _, path := range paths {
		analysis, analyzeErr := a.AnalyzeFile(path)
		if analyzeErr != nil {
			return nil, analyzeErr
		}

		results = append(results, analysis)
	}

	return results, nil
}

/*
|--------------------------------------------------------------------------
| 分析指定本地模型文件
|--------------------------------------------------------------------------
| 根据文件扩展名选择分析流程，返回格式、预览能力和基础几何信息。
|--------------------------------------------------------------------------
*/
func (a *ModelAnalyzer) AnalyzeFile(path string) (ModelAnalysis, error) {
	if strings.TrimSpace(path) == "" {
		return ModelAnalysis{}, ErrEmptyModelPath
	}

	info, err := os.Stat(path)
	if err != nil {
		return ModelAnalysis{}, fmt.Errorf("读取模型文件信息失败 %s: %w", path, err)
	}

	extension := getModelExtension(path)
	format, ok := supportedModelExtensions[extension]
	if !ok {
		return ModelAnalysis{}, fmt.Errorf("%w: .%s", ErrUnsupportedModelFile, extension)
	}

	analysis := ModelAnalysis{
		ID:              buildModelAnalysisID(path, info),
		Path:            path,
		Name:            filepath.Base(path),
		Extension:       extension,
		Size:            info.Size(),
		SizeLabel:       formatModelFileSize(info.Size()),
		FormatName:      format.Name,
		FormatFamily:    format.Family,
		Previewable:     format.Previewable,
		NeedsConversion: format.NeedsConversion,
		PreviewFormat:   format.PreviewFormat,
		Details: []AnalysisDetail{
			{Label: "文件大小", Value: formatModelFileSize(info.Size())},
			{Label: "本地路径", Value: path},
		},
	}

	if err := enrichModelAnalysis(path, &analysis); err != nil {
		return ModelAnalysis{}, err
	}

	return analysis, nil
}

/*
|--------------------------------------------------------------------------
| 读取可预览模型文件
|--------------------------------------------------------------------------
| 读取 STL、OBJ、FBX 本地文件，并以 Base64 返回给前端创建预览 Blob。
|--------------------------------------------------------------------------
*/
func (a *ModelAnalyzer) ReadPreviewModel(path string) (PreviewModelPayload, error) {
	analysis, err := a.AnalyzeFile(path)
	if err != nil {
		return PreviewModelPayload{}, err
	}

	if !analysis.Previewable {
		return PreviewModelPayload{}, ErrPreviewNeedsConvert
	}

	if analysis.Size > maxPreviewFileBytes {
		return PreviewModelPayload{}, ErrPreviewFileTooLarge
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return PreviewModelPayload{}, fmt.Errorf("读取预览模型文件失败 %s: %w", path, err)
	}

	return PreviewModelPayload{
		FileName:  analysis.Name,
		Extension: analysis.Extension,
		MimeType:  getPreviewMimeType(analysis.Extension),
		Base64:    base64.StdEncoding.EncodeToString(content),
	}, nil
}

/*
|--------------------------------------------------------------------------
| 读取上传模型文件
|--------------------------------------------------------------------------
| 读取任意受支持模型文件，并以 Base64 返回给前端上传云端，不写入本地副本。
|--------------------------------------------------------------------------
*/
func (a *ModelAnalyzer) ReadModelForUpload(path string) (UploadModelPayload, error) {
	analysis, err := a.AnalyzeFile(path)
	if err != nil {
		return UploadModelPayload{}, err
	}

	if analysis.Size > maxUploadFileBytes {
		return UploadModelPayload{}, fmt.Errorf("模型文件超过上传上限 %s", formatModelFileSize(maxUploadFileBytes))
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return UploadModelPayload{}, fmt.Errorf("读取上传模型文件失败 %s: %w", path, err)
	}

	return UploadModelPayload{
		FileName:  analysis.Name,
		Extension: analysis.Extension,
		Size:      analysis.Size,
		MimeType:  getPreviewMimeType(analysis.Extension),
		Base64:    base64.StdEncoding.EncodeToString(content),
	}, nil
}

/*
|--------------------------------------------------------------------------
| 模型格式专属分析
|--------------------------------------------------------------------------
| 根据格式补充 STL、OBJ、FBX、STEP 和 SolidWorks 的本地分析详情。
|--------------------------------------------------------------------------
*/
func enrichModelAnalysis(path string, analysis *ModelAnalysis) error {
	switch analysis.Extension {
	case "stl":
		return analyzeSTL(path, analysis)
	case "obj":
		return analyzeOBJ(path, analysis)
	case "fbx":
		return analyzeFBX(path, analysis)
	case "stp", "step":
		return analyzeSTEP(path, analysis)
	case "sldprt", "sldasm", "slddrw":
		analyzeSolidWorks(analysis)
		return nil
	default:
		return fmt.Errorf("%w: .%s", ErrUnsupportedModelFile, analysis.Extension)
	}
}

func analyzeSTL(path string, analysis *ModelAnalysis) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 STL 模型失败 %s: %w", path, err)
	}

	if isBinarySTL(content) {
		return analyzeBinarySTL(content, analysis)
	}

	return analyzeASCIISTL(content, analysis)
}

func analyzeBinarySTL(content []byte, analysis *ModelAnalysis) error {
	if len(content) < 84 {
		return errors.New("二进制 STL 文件结构不完整")
	}

	triangleCount := int(binary.LittleEndian.Uint32(content[80:84]))
	expectedSize := 84 + triangleCount*50
	bounds := newEmptyBounds()

	for offset := 84; offset+50 <= len(content); offset += 50 {
		for vertexIndex := 0; vertexIndex < 3; vertexIndex++ {
			vertexOffset := offset + 12 + vertexIndex*12

			x := float64(math.Float32frombits(binary.LittleEndian.Uint32(content[vertexOffset : vertexOffset+4])))
			y := float64(math.Float32frombits(binary.LittleEndian.Uint32(content[vertexOffset+4 : vertexOffset+8])))
			z := float64(math.Float32frombits(binary.LittleEndian.Uint32(content[vertexOffset+8 : vertexOffset+12])))

			bounds.include(x, y, z)
		}
	}

	analysis.Bounds = bounds.toModelBounds()
	analysis.Summary = fmt.Sprintf("二进制 STL，可直接预览，包含约 %d 个三角面。", triangleCount)
	analysis.Details = append(analysis.Details,
		AnalysisDetail{Label: "STL 类型", Value: "二进制"},
		AnalysisDetail{Label: "三角面数量", Value: strconv.Itoa(triangleCount)},
		AnalysisDetail{Label: "文件结构", Value: formatSTLStructureStatus(len(content), expectedSize)},
	)

	return nil
}

func analyzeASCIISTL(content []byte, analysis *ModelAnalysis) error {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	vertexCount := 0
	facetCount := 0
	bounds := newEmptyBounds()
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "facet normal") {
			facetCount++
			continue
		}

		if strings.HasPrefix(line, "vertex ") {
			parts := strings.Fields(line)
			if len(parts) != 4 {
				continue
			}

			x, xErr := strconv.ParseFloat(parts[1], 64)
			y, yErr := strconv.ParseFloat(parts[2], 64)
			z, zErr := strconv.ParseFloat(parts[3], 64)
			if xErr == nil && yErr == nil && zErr == nil {
				vertexCount++
				bounds.include(x, y, z)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("分析 ASCII STL 失败: %w", err)
	}

	if facetCount == 0 {
		facetCount = vertexCount / 3
	}

	analysis.Bounds = bounds.toModelBounds()
	analysis.Summary = fmt.Sprintf("ASCII STL，可直接预览，包含约 %d 个三角面。", facetCount)
	analysis.Details = append(analysis.Details,
		AnalysisDetail{Label: "STL 类型", Value: "ASCII"},
		AnalysisDetail{Label: "顶点数量", Value: strconv.Itoa(vertexCount)},
		AnalysisDetail{Label: "三角面数量", Value: strconv.Itoa(facetCount)},
	)

	return nil
}

func analyzeOBJ(path string, analysis *ModelAnalysis) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("读取 OBJ 模型失败 %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	vertexCount := 0
	faceCount := 0
	objectCount := 0
	groupCount := 0
	bounds := newEmptyBounds()
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		switch {
		case strings.HasPrefix(line, "v "):
			vertexCount++
			includeOBJVertex(line, bounds)
		case strings.HasPrefix(line, "f "):
			faceCount++
		case strings.HasPrefix(line, "o "):
			objectCount++
		case strings.HasPrefix(line, "g "):
			groupCount++
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("分析 OBJ 模型失败: %w", err)
	}

	analysis.Bounds = bounds.toModelBounds()
	analysis.Summary = fmt.Sprintf("OBJ 网格模型，可直接预览，包含 %d 个顶点和 %d 个面。", vertexCount, faceCount)
	analysis.Details = append(analysis.Details,
		AnalysisDetail{Label: "顶点数量", Value: strconv.Itoa(vertexCount)},
		AnalysisDetail{Label: "面数量", Value: strconv.Itoa(faceCount)},
		AnalysisDetail{Label: "对象数量", Value: strconv.Itoa(objectCount)},
		AnalysisDetail{Label: "分组数量", Value: strconv.Itoa(groupCount)},
	)

	return nil
}

func analyzeFBX(path string, analysis *ModelAnalysis) error {
	header, err := readFileHeader(path, 64)
	if err != nil {
		return fmt.Errorf("读取 FBX 模型头失败 %s: %w", path, err)
	}

	fbxType := "ASCII"
	if bytes.HasPrefix(header, []byte("Kaydara FBX Binary")) {
		fbxType = "二进制"
	}

	analysis.Summary = fmt.Sprintf("%s FBX 场景文件，可直接交给 Three.js 预览。", fbxType)
	analysis.Details = append(analysis.Details,
		AnalysisDetail{Label: "FBX 类型", Value: fbxType},
		AnalysisDetail{Label: "预览方式", Value: "前端 Three.js FBXLoader"},
	)

	return nil
}

func analyzeSTEP(path string, analysis *ModelAnalysis) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("读取 STEP 模型失败 %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	entityCount := 0
	productName := ""
	scanner.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		upperLine := strings.ToUpper(line)

		if strings.HasPrefix(line, "#") && strings.Contains(line, "=") {
			entityCount++
		}

		if productName == "" && strings.Contains(upperLine, "PRODUCT(") {
			productName = extractSTEPQuotedValue(line)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("分析 STEP 模型失败: %w", err)
	}

	analysis.Summary = "STEP/STP CAD 模型已完成本地结构识别，需要转换为 STL 或 OBJ 后预览。"
	analysis.Details = append(analysis.Details,
		AnalysisDetail{Label: "STEP 实体数量", Value: strconv.Itoa(entityCount)},
		AnalysisDetail{Label: "预览状态", Value: "需要服务端转换"},
	)

	if productName != "" {
		analysis.Details = append(analysis.Details, AnalysisDetail{Label: "产品名称", Value: productName})
	}

	return nil
}

func analyzeSolidWorks(analysis *ModelAnalysis) {
	analysis.Summary = "SolidWorks 官方格式已识别，需要通过本地服务端或外部转换器导出 STL/OBJ 后预览。"
	analysis.Details = append(analysis.Details,
		AnalysisDetail{Label: "格式来源", Value: "SolidWorks 官方文件"},
		AnalysisDetail{Label: "预览状态", Value: "需要服务端转换"},
	)
}

/*
|--------------------------------------------------------------------------
| 共用分析工具函数
|--------------------------------------------------------------------------
| 放置格式识别、包围盒、文件头读取、大小格式化等多个分析流程复用的能力。
|--------------------------------------------------------------------------
*/
type modelBoundsBuilder struct {
	hasValue bool
	minX     float64
	minY     float64
	minZ     float64
	maxX     float64
	maxY     float64
	maxZ     float64
}

func newEmptyBounds() *modelBoundsBuilder {
	return &modelBoundsBuilder{
		minX: math.Inf(1),
		minY: math.Inf(1),
		minZ: math.Inf(1),
		maxX: math.Inf(-1),
		maxY: math.Inf(-1),
		maxZ: math.Inf(-1),
	}
}

func (b *modelBoundsBuilder) include(x float64, y float64, z float64) {
	b.hasValue = true
	b.minX = math.Min(b.minX, x)
	b.minY = math.Min(b.minY, y)
	b.minZ = math.Min(b.minZ, z)
	b.maxX = math.Max(b.maxX, x)
	b.maxY = math.Max(b.maxY, y)
	b.maxZ = math.Max(b.maxZ, z)
}

func (b *modelBoundsBuilder) toModelBounds() *ModelBounds {
	if !b.hasValue {
		return nil
	}

	return &ModelBounds{
		MinX: b.minX,
		MinY: b.minY,
		MinZ: b.minZ,
		MaxX: b.maxX,
		MaxY: b.maxY,
		MaxZ: b.maxZ,
	}
}

func includeOBJVertex(line string, bounds *modelBoundsBuilder) {
	parts := strings.Fields(line)
	if len(parts) < 4 {
		return
	}

	x, xErr := strconv.ParseFloat(parts[1], 64)
	y, yErr := strconv.ParseFloat(parts[2], 64)
	z, zErr := strconv.ParseFloat(parts[3], 64)
	if xErr == nil && yErr == nil && zErr == nil {
		bounds.include(x, y, z)
	}
}

func isBinarySTL(content []byte) bool {
	if len(content) < 84 {
		return false
	}

	triangleCount := int(binary.LittleEndian.Uint32(content[80:84]))
	expectedSize := 84 + triangleCount*50

	if expectedSize == len(content) {
		return true
	}

	header := strings.ToLower(strings.TrimSpace(string(content[:minInt(len(content), 80)])))

	return !strings.HasPrefix(header, "solid")
}

func readFileHeader(path string, length int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	buffer := make([]byte, length)
	count, err := file.Read(buffer)
	if err != nil && count == 0 {
		return nil, err
	}

	return buffer[:count], nil
}

func extractSTEPQuotedValue(line string) string {
	firstQuote := strings.Index(line, "'")
	if firstQuote < 0 {
		return ""
	}

	rest := line[firstQuote+1:]
	secondQuote := strings.Index(rest, "'")
	if secondQuote < 0 {
		return ""
	}

	return rest[:secondQuote]
}

func getModelExtension(path string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
}

func buildModelAnalysisID(path string, info os.FileInfo) string {
	return fmt.Sprintf(
		"%s-%d-%d",
		filepath.Base(path),
		info.Size(),
		info.ModTime().UnixNano(),
	)
}

func formatModelFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}

	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}

	return fmt.Sprintf("%.1f MB", float64(size)/1024/1024)
}

func formatSTLStructureStatus(actualSize int, expectedSize int) string {
	if actualSize == expectedSize {
		return "尺寸匹配"
	}

	return fmt.Sprintf("尺寸不完全匹配，实际 %d B，预期 %d B", actualSize, expectedSize)
}

func getPreviewMimeType(extension string) string {
	switch extension {
	case "stl":
		return "model/stl"
	case "obj":
		return "text/plain"
	case "fbx":
		return "application/octet-stream"
	default:
		return "application/octet-stream"
	}
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}

	return b
}
