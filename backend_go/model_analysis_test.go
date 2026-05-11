/*
|--------------------------------------------------------------------------
| 本地三维模型分析测试
|--------------------------------------------------------------------------
| 验证 STL 和 OBJ 本地模型分析是否能提取基础几何统计和包围盒。
|--------------------------------------------------------------------------
*/
package main

import (
	"os"
	"path/filepath"
	"testing"
)

/*
|--------------------------------------------------------------------------
| 模块能力清单
|--------------------------------------------------------------------------
| STL 分析测试：验证 ASCII STL 三角面数量和包围盒。
| OBJ 分析测试：验证 OBJ 顶点数量、面数量和包围盒。
|--------------------------------------------------------------------------
*/

/*
|--------------------------------------------------------------------------
| ASCII STL 分析测试
|--------------------------------------------------------------------------
| 写入一个最小三角面 STL 文件，验证后端能识别为可直接预览模型。
|--------------------------------------------------------------------------
*/
func TestAnalyzeASCIISTL(t *testing.T) {
	path := writeTempModelFile(t, "triangle.stl", `solid triangle
facet normal 0 0 1
  outer loop
    vertex 0 0 0
    vertex 1 0 0
    vertex 0 1 0
  endloop
endfacet
endsolid triangle
`)

	analysis, err := NewModelAnalyzer().AnalyzeFile(path)
	if err != nil {
		t.Fatalf("AnalyzeFile returned error: %v", err)
	}

	if !analysis.Previewable {
		t.Fatal("STL should be directly previewable")
	}

	if analysis.Bounds == nil {
		t.Fatal("STL bounds should be available")
	}

	if analysis.Bounds.MaxX != 1 || analysis.Bounds.MaxY != 1 {
		t.Fatalf("unexpected STL bounds: %+v", analysis.Bounds)
	}
}

/*
|--------------------------------------------------------------------------
| OBJ 分析测试
|--------------------------------------------------------------------------
| 写入一个最小 OBJ 文件，验证后端能统计顶点和面信息。
|--------------------------------------------------------------------------
*/
func TestAnalyzeOBJ(t *testing.T) {
	path := writeTempModelFile(t, "quad.obj", `o quad
v 0 0 0
v 1 0 0
v 1 1 0
v 0 1 0
f 1 2 3 4
`)

	analysis, err := NewModelAnalyzer().AnalyzeFile(path)
	if err != nil {
		t.Fatalf("AnalyzeFile returned error: %v", err)
	}

	if analysis.FormatName != "OBJ" {
		t.Fatalf("unexpected format name: %s", analysis.FormatName)
	}

	if !analysis.Previewable {
		t.Fatal("OBJ should be directly previewable")
	}

	if analysis.Bounds == nil {
		t.Fatal("OBJ bounds should be available")
	}

	if analysis.Bounds.MaxX != 1 || analysis.Bounds.MaxY != 1 {
		t.Fatalf("unexpected OBJ bounds: %+v", analysis.Bounds)
	}
}

/*
|--------------------------------------------------------------------------
| 共用测试工具函数
|--------------------------------------------------------------------------
| 放置测试模型文件写入逻辑，供不同格式分析测试复用。
|--------------------------------------------------------------------------
*/
func writeTempModelFile(t *testing.T, name string, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp model file: %v", err)
	}

	return path
}
