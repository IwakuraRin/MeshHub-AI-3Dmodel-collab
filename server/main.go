/*
|--------------------------------------------------------------------------
| 资源转码云端服务入口
|--------------------------------------------------------------------------
| 提供账户管理、本地管理前端、客户端登录、用户模型库和格式转换接口。
|--------------------------------------------------------------------------
*/
package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

/*
|--------------------------------------------------------------------------
| 模块能力清单
|--------------------------------------------------------------------------
| 管理前端：只允许服务器本机访问，用于创建和删除账户。
| 账户认证：客户端必须使用云端账户密码登录后才允许使用。
| 用户数据库：每个账户都有自己的 SQLite 数据库，用于保存三维模型。
| 模型接口：支持上传、列表、下载，并预留系统格式转换命令调用。
|--------------------------------------------------------------------------
*/

/*
|--------------------------------------------------------------------------
| 服务端配置
|--------------------------------------------------------------------------
| 控制监听地址、数据目录、主数据库、用户数据库和可选格式转换命令。
|--------------------------------------------------------------------------
*/
const (
	defaultListenAddress = "127.0.0.1:8787"
	defaultAdminAddress  = "127.0.0.1:8788"
	defaultDataDirectory = "data"
)

/*
|--------------------------------------------------------------------------
| 服务端应用结构
|--------------------------------------------------------------------------
| 保存主数据库、数据目录、会话令牌和运行时配置。
|--------------------------------------------------------------------------
*/
type ServerApp struct {
	db        *sql.DB
	dataDir   string
	userDBDir string
	converter string

	sessionsMu sync.RWMutex
	sessions   map[string]string

	workerStop chan struct{}
}

/*
|--------------------------------------------------------------------------
| 账户记录
|--------------------------------------------------------------------------
| 管理主数据库中的账户信息，密码只保存 bcrypt 哈希。
|--------------------------------------------------------------------------
*/
type Account struct {
	Username  string `json:"username"`
	CreatedAt string `json:"createdAt"`
}

/*
|--------------------------------------------------------------------------
| 登录请求和响应
|--------------------------------------------------------------------------
| 客户端登录成功后获得令牌，后续请求通过 Authorization Bearer 携带。
|--------------------------------------------------------------------------
*/
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

/*
|--------------------------------------------------------------------------
| 用户模型记录
|--------------------------------------------------------------------------
| 每个账户自己的数据库中保存模型文件和基础元数据。
|--------------------------------------------------------------------------
*/
type CloudModel struct {
	ID            int64  `json:"id"`
	FileName      string `json:"fileName"`
	Extension     string `json:"extension"`
	Size          int64  `json:"size"`
	SizeLabel     string `json:"sizeLabel"`
	Previewable   bool   `json:"previewable"`
	NeedsConvert  bool   `json:"needsConvert"`
	ConvertedFrom int64  `json:"convertedFrom,omitempty"`
	CreatedAt     string `json:"createdAt"`
}

/*
|--------------------------------------------------------------------------
| 分片上传响应
|--------------------------------------------------------------------------
| 大模型通过多次小请求上传，最后一片到达后返回写入数据库的模型记录。
|--------------------------------------------------------------------------
*/
type ChunkUploadResponse struct {
	Status   string      `json:"status"`
	Message  string      `json:"message"`
	Progress int         `json:"progress"`
	Model    *CloudModel `json:"model,omitempty"`
}

/*
|--------------------------------------------------------------------------
| 格式转换请求和响应
|--------------------------------------------------------------------------
| 客户端请求服务端调用系统转换器，未配置转换器时返回明确的等待配置状态。
|--------------------------------------------------------------------------
*/
type ConvertRequest struct {
	ModelID      int64  `json:"modelId"`
	TargetFormat string `json:"targetFormat"`
}

type ConvertResponse struct {
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Model   *CloudModel    `json:"model,omitempty"`
	Job     *ConversionJob `json:"job,omitempty"`
}

/*
|--------------------------------------------------------------------------
| 转换任务记录
|--------------------------------------------------------------------------
| 保存服务端后台转换任务状态，客户端通过任务 ID 查询进度和结果模型。
|--------------------------------------------------------------------------
*/
type ConversionJob struct {
	ID           int64       `json:"id"`
	Username     string      `json:"username"`
	ModelID      int64       `json:"modelId"`
	TargetFormat string      `json:"targetFormat"`
	Status       string      `json:"status"`
	Progress     int         `json:"progress"`
	Message      string      `json:"message"`
	Error        string      `json:"error,omitempty"`
	ResultModel  *CloudModel `json:"resultModel,omitempty"`
	ResultID     int64       `json:"resultId,omitempty"`
	CreatedAt    string      `json:"createdAt"`
	UpdatedAt    string      `json:"updatedAt"`
}

/*
|--------------------------------------------------------------------------
| 服务端创建
|--------------------------------------------------------------------------
| 初始化主数据库目录、用户数据库目录和会话存储。
|--------------------------------------------------------------------------
*/
func NewServerApp(dataDir string) (*ServerApp, error) {
	if dataDir == "" {
		dataDir = defaultDataDirectory
	}

	userDBDir := filepath.Join(dataDir, "user_dbs")
	if err := os.MkdirAll(userDBDir, 0755); err != nil {
		return nil, fmt.Errorf("创建用户数据库目录失败: %w", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(dataDir, "server.db"))
	if err != nil {
		return nil, fmt.Errorf("打开主数据库失败: %w", err)
	}

	app := &ServerApp{
		db:         db,
		dataDir:    dataDir,
		userDBDir:  userDBDir,
		converter:  os.Getenv("ASSET_TRANSCODER_CONVERTER"),
		sessions:   map[string]string{},
		workerStop: make(chan struct{}),
	}

	if err := app.migrateMainDatabase(); err != nil {
		return nil, err
	}
	if err := app.resetRunningConversionJobs(); err != nil {
		return nil, err
	}
	go app.runConversionWorker()

	return app, nil
}

/*
|--------------------------------------------------------------------------
| 服务端启动流程
|--------------------------------------------------------------------------
| 读取监听地址，注册路由，并启动 HTTP 服务。
|--------------------------------------------------------------------------
*/
func main() {
	listenAddress := os.Getenv("ASSET_TRANSCODER_SERVER_ADDR")
	if listenAddress == "" {
		listenAddress = defaultListenAddress
	}
	adminAddress := os.Getenv("ASSET_TRANSCODER_ADMIN_ADDR")
	if adminAddress == "" {
		adminAddress = defaultAdminAddress
	}

	app, err := NewServerApp(defaultDataDirectory)
	if err != nil {
		log.Fatal(err)
	}
	defer app.db.Close()

	server := &http.Server{
		Addr:              listenAddress,
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	adminServer := &http.Server{
		Addr:              adminAddress,
		Handler:           app.adminRoutes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("MeshHub admin listening on http://%s/admin", adminAddress)
		if err := adminServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	log.Printf("MeshHub public api listening on http://%s", listenAddress)
	log.Fatal(server.ListenAndServe())
}

/*
|--------------------------------------------------------------------------
| HTTP 路由
|--------------------------------------------------------------------------
| 公网端口只注册客户端 API；管理页放到独立本机端口，避免被 frp/relay 暴露。
|--------------------------------------------------------------------------
*/
func (a *ServerApp) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", a.withCORS(a.handleHealth))
	mux.HandleFunc("/api/auth/login", a.withCORS(a.handleLogin))
	mux.HandleFunc("/api/client/models", a.withCORS(a.requireAuth(a.handleClientModels)))
	mux.HandleFunc("/api/client/models/chunk", a.withCORS(a.requireAuth(a.handleChunkUploadModel)))
	mux.HandleFunc("/api/client/models/download", a.withCORS(a.requireAuth(a.handleModelDownload)))
	mux.HandleFunc("/api/client/convert", a.withCORS(a.requireAuth(a.handleConvertModel)))
	mux.HandleFunc("/api/client/convert/status", a.withCORS(a.requireAuth(a.handleConvertStatus)))

	return mux
}

/*
|--------------------------------------------------------------------------
| 管理端路由
|--------------------------------------------------------------------------
| 账户创建和删除只注册在 127.0.0.1 管理端口，公网 API 端口不暴露这些路由。
|--------------------------------------------------------------------------
*/
func (a *ServerApp) adminRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/admin", a.localOnly(a.handleAdminPage))
	mux.HandleFunc("/api/admin/accounts", a.localOnly(a.handleAdminAccounts))

	return mux
}

/*
|--------------------------------------------------------------------------
| 主数据库迁移
|--------------------------------------------------------------------------
| 创建账户表，用于保存账户名、密码哈希和创建时间。
|--------------------------------------------------------------------------
*/
func (a *ServerApp) migrateMainDatabase() error {
	if err := os.MkdirAll(a.dataDir, 0755); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}

	_, err := a.db.Exec(`
		CREATE TABLE IF NOT EXISTS accounts (
			username TEXT PRIMARY KEY,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS user_info_pool (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			action TEXT NOT NULL,
			created_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS model_data_pool (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			model_id INTEGER NOT NULL,
			file_name TEXT NOT NULL,
			extension TEXT NOT NULL,
			action TEXT NOT NULL,
			created_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS conversion_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			model_id INTEGER NOT NULL,
			target_format TEXT NOT NULL,
			status TEXT NOT NULL,
			progress INTEGER NOT NULL,
			message TEXT NOT NULL,
			error TEXT NOT NULL DEFAULT '',
			result_model_id INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
	`)
	if err != nil {
		return fmt.Errorf("迁移主数据库失败: %w", err)
	}

	return nil
}

/*
|--------------------------------------------------------------------------
| 本机管理页面
|--------------------------------------------------------------------------
| 提供极简账户创建和删除界面，只允许服务器本机访问。
|--------------------------------------------------------------------------
*/
func (a *ServerApp) handleAdminPage(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "请求方法不允许")
		return
	}

	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = writer.Write([]byte(adminHTML))
}

func (a *ServerApp) handleAdminAccounts(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		a.listAccounts(writer)
	case http.MethodPost:
		a.createAccount(writer, request)
	case http.MethodDelete:
		a.deleteAccount(writer, request)
	default:
		writeError(writer, http.StatusMethodNotAllowed, "请求方法不允许")
	}
}

func (a *ServerApp) listAccounts(writer http.ResponseWriter) {
	rows, err := a.db.Query(`SELECT username, created_at FROM accounts ORDER BY created_at DESC`)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "读取账户列表失败")
		return
	}
	defer rows.Close()

	accounts := []Account{}
	for rows.Next() {
		var account Account
		if err := rows.Scan(&account.Username, &account.CreatedAt); err != nil {
			writeError(writer, http.StatusInternalServerError, "解析账户列表失败")
			return
		}

		accounts = append(accounts, account)
	}

	writeJSON(writer, http.StatusOK, accounts)
}

func (a *ServerApp) createAccount(writer http.ResponseWriter, request *http.Request) {
	var payload LoginRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(writer, http.StatusBadRequest, "账户请求格式错误")
		return
	}

	username := normalizeUsername(payload.Username)
	if username == "" || payload.Password == "" {
		writeError(writer, http.StatusBadRequest, "账户和密码不能为空")
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "生成密码哈希失败")
		return
	}

	_, err = a.db.Exec(
		`INSERT INTO accounts (username, password_hash, created_at) VALUES (?, ?, ?)`,
		username,
		string(passwordHash),
		time.Now().Format(time.RFC3339),
	)
	if err != nil {
		writeError(writer, http.StatusConflict, "账户已存在或创建失败")
		return
	}

	if err := a.ensureUserDatabase(username); err != nil {
		writeError(writer, http.StatusInternalServerError, "创建用户数据库失败")
		return
	}

	_ = a.recordUserPoolEvent(username, "create_account")

	writeJSON(writer, http.StatusCreated, Account{
		Username:  username,
		CreatedAt: time.Now().Format(time.RFC3339),
	})
}

func (a *ServerApp) deleteAccount(writer http.ResponseWriter, request *http.Request) {
	username := normalizeUsername(request.URL.Query().Get("username"))
	if username == "" {
		writeError(writer, http.StatusBadRequest, "账户不能为空")
		return
	}

	_, err := a.db.Exec(`DELETE FROM accounts WHERE username = ?`, username)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "删除账户失败")
		return
	}

	_ = os.Remove(a.userDatabasePath(username))
	_ = a.recordUserPoolEvent(username, "delete_account")
	writeJSON(writer, http.StatusOK, map[string]string{"status": "deleted"})
}

/*
|--------------------------------------------------------------------------
| 客户端健康检查和登录
|--------------------------------------------------------------------------
| 客户端启动时先检查服务可达，随后强制输入账户密码登录。
|--------------------------------------------------------------------------
*/
func (a *ServerApp) handleHealth(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "请求方法不允许")
		return
	}

	writeJSON(writer, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": "0.1.0",
	})
}

func (a *ServerApp) handleLogin(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "请求方法不允许")
		return
	}

	var payload LoginRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(writer, http.StatusBadRequest, "登录请求格式错误")
		return
	}

	username := normalizeUsername(payload.Username)
	passwordHash, err := a.findPasswordHash(username)
	if err != nil {
		writeError(writer, http.StatusUnauthorized, "账户或密码错误")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(payload.Password)); err != nil {
		writeError(writer, http.StatusUnauthorized, "账户或密码错误")
		return
	}

	token, err := createToken()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "创建登录令牌失败")
		return
	}

	a.sessionsMu.Lock()
	a.sessions[token] = username
	a.sessionsMu.Unlock()

	writeJSON(writer, http.StatusOK, LoginResponse{
		Token:    token,
		Username: username,
	})
}

/*
|--------------------------------------------------------------------------
| 客户端模型库接口
|--------------------------------------------------------------------------
| 登录用户可以上传模型到自己的数据库，也可以拉取云端模型列表。
|--------------------------------------------------------------------------
*/
func (a *ServerApp) handleClientModels(writer http.ResponseWriter, request *http.Request, username string) {
	switch request.Method {
	case http.MethodGet:
		a.listUserModels(writer, username)
	case http.MethodPost:
		a.uploadUserModel(writer, request, username)
	default:
		writeError(writer, http.StatusMethodNotAllowed, "请求方法不允许")
	}
}

func (a *ServerApp) listUserModels(writer http.ResponseWriter, username string) {
	userDB, err := a.openUserDatabase(username)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "打开用户数据库失败")
		return
	}
	defer userDB.Close()

	rows, err := userDB.Query(`SELECT id, file_name, extension, size, converted_from, created_at FROM models ORDER BY created_at DESC`)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "读取用户模型失败")
		return
	}
	defer rows.Close()

	models := []CloudModel{}
	for rows.Next() {
		var model CloudModel
		if err := rows.Scan(
			&model.ID,
			&model.FileName,
			&model.Extension,
			&model.Size,
			&model.ConvertedFrom,
			&model.CreatedAt,
		); err != nil {
			writeError(writer, http.StatusInternalServerError, "解析用户模型失败")
			return
		}

		model.SizeLabel = formatBytes(model.Size)
		model.Previewable = isPreviewableExtension(model.Extension)
		model.NeedsConvert = !model.Previewable

		models = append(models, model)
	}

	writeJSON(writer, http.StatusOK, models)
}

func (a *ServerApp) uploadUserModel(writer http.ResponseWriter, request *http.Request, username string) {
	if err := request.ParseMultipartForm(256 << 20); err != nil {
		writeError(writer, http.StatusBadRequest, "上传模型表单过大或格式错误")
		return
	}

	file, header, err := request.FormFile("model")
	if err != nil {
		writeError(writer, http.StatusBadRequest, "缺少模型文件")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "读取上传模型失败")
		return
	}

	model, err := a.saveUserModel(username, header, content)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "保存用户模型失败")
		return
	}

	_ = a.recordModelPoolEvent(username, model, "upload")

	writeJSON(writer, http.StatusCreated, model)
}

/*
|--------------------------------------------------------------------------
| 分片上传模型
|--------------------------------------------------------------------------
| 大模型文件按小片段传输，服务端追加到临时文件，最后一片到达后写入用户数据库。
|--------------------------------------------------------------------------
*/
func (a *ServerApp) handleChunkUploadModel(writer http.ResponseWriter, request *http.Request, username string) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "请求方法不允许")
		return
	}

	if err := request.ParseMultipartForm(8 << 20); err != nil {
		writeError(writer, http.StatusBadRequest, "上传分片过大或格式错误")
		return
	}

	uploadID := normalizeUploadID(request.FormValue("uploadId"))
	fileName := filepath.Base(request.FormValue("fileName"))
	chunkIndex, indexErr := strconv.Atoi(request.FormValue("chunkIndex"))
	totalChunks, totalErr := strconv.Atoi(request.FormValue("totalChunks"))
	if uploadID == "" || fileName == "." || fileName == "" || indexErr != nil || totalErr != nil || totalChunks <= 0 {
		writeError(writer, http.StatusBadRequest, "分片上传参数错误")
		return
	}
	if chunkIndex < 0 || chunkIndex >= totalChunks {
		writeError(writer, http.StatusBadRequest, "分片序号超出范围")
		return
	}

	chunk, _, err := request.FormFile("chunk")
	if err != nil {
		writeError(writer, http.StatusBadRequest, "缺少上传分片")
		return
	}
	defer chunk.Close()

	model, progress, err := a.appendUserModelChunk(username, uploadID, fileName, chunkIndex, totalChunks, chunk)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}

	response := ChunkUploadResponse{
		Status:   "uploading",
		Message:  "模型分片已上传。",
		Progress: progress,
	}
	if model != nil {
		response.Status = "done"
		response.Message = "模型上传完成。"
		response.Model = model
	}

	writeJSON(writer, http.StatusOK, response)
}

func (a *ServerApp) handleModelDownload(writer http.ResponseWriter, request *http.Request, username string) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "请求方法不允许")
		return
	}

	modelID := request.URL.Query().Get("id")
	if modelID == "" {
		writeError(writer, http.StatusBadRequest, "模型 ID 不能为空")
		return
	}

	userDB, err := a.openUserDatabase(username)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "打开用户数据库失败")
		return
	}
	defer userDB.Close()

	var fileName string
	var content []byte
	err = userDB.QueryRow(`SELECT file_name, content FROM models WHERE id = ?`, modelID).Scan(&fileName, &content)
	if err != nil {
		writeError(writer, http.StatusNotFound, "模型不存在")
		return
	}

	writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	writer.Header().Set("Content-Type", "application/octet-stream")
	_, _ = writer.Write(content)
}

/*
|--------------------------------------------------------------------------
| 格式转换接口
|--------------------------------------------------------------------------
| 调用系统转换器处理三维格式转换；未配置转换器时返回待配置状态。
|--------------------------------------------------------------------------
*/
func (a *ServerApp) handleConvertModel(writer http.ResponseWriter, request *http.Request, username string) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "请求方法不允许")
		return
	}

	var payload ConvertRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(writer, http.StatusBadRequest, "转换请求格式错误")
		return
	}

	targetFormat := normalizeExtension(payload.TargetFormat)
	if payload.ModelID <= 0 || targetFormat == "" {
		writeError(writer, http.StatusBadRequest, "模型 ID 和目标格式不能为空")
		return
	}

	if !isSupportedTargetFormat(targetFormat) {
		writeError(writer, http.StatusBadRequest, "暂不支持该目标格式")
		return
	}

	if _, _, err := a.findUserModelContent(username, payload.ModelID); err != nil {
		writeError(writer, http.StatusNotFound, "待转换模型不存在")
		return
	}

	job, err := a.createConversionJob(username, payload.ModelID, targetFormat)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "创建转换任务失败")
		return
	}

	writeJSON(writer, http.StatusAccepted, ConvertResponse{
		Status:  "queued",
		Message: "转换任务已进入服务端队列。",
		Job:     &job,
	})
}

func (a *ServerApp) handleConvertStatus(writer http.ResponseWriter, request *http.Request, username string) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "请求方法不允许")
		return
	}

	jobID, err := strconv.ParseInt(request.URL.Query().Get("id"), 10, 64)
	if err != nil || jobID <= 0 {
		writeError(writer, http.StatusBadRequest, "转换任务 ID 无效")
		return
	}

	job, err := a.findConversionJob(username, jobID)
	if err != nil {
		writeError(writer, http.StatusNotFound, "转换任务不存在")
		return
	}

	writeJSON(writer, http.StatusOK, ConvertResponse{
		Status:  job.Status,
		Message: job.Message,
		Job:     &job,
	})
}

/*
|--------------------------------------------------------------------------
| 用户数据库
|--------------------------------------------------------------------------
| 每个账户对应一个独立 SQLite 数据库，用于存放该账户的云端三维模型。
|--------------------------------------------------------------------------
*/
func (a *ServerApp) ensureUserDatabase(username string) error {
	userDB, err := a.openUserDatabase(username)
	if err != nil {
		return err
	}
	defer userDB.Close()

	return nil
}

func (a *ServerApp) openUserDatabase(username string) (*sql.DB, error) {
	userDB, err := sql.Open("sqlite", a.userDatabasePath(username))
	if err != nil {
		return nil, err
	}

	_, err = userDB.Exec(`
		CREATE TABLE IF NOT EXISTS models (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_name TEXT NOT NULL,
			extension TEXT NOT NULL,
			size INTEGER NOT NULL,
			content BLOB NOT NULL,
			converted_from INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		);
	`)
	if err != nil {
		_ = userDB.Close()
		return nil, err
	}

	_, _ = userDB.Exec(`ALTER TABLE models ADD COLUMN converted_from INTEGER NOT NULL DEFAULT 0`)

	return userDB, nil
}

func (a *ServerApp) saveUserModel(username string, header *multipart.FileHeader, content []byte) (CloudModel, error) {
	userDB, err := a.openUserDatabase(username)
	if err != nil {
		return CloudModel{}, err
	}
	defer userDB.Close()

	extension := normalizeExtension(filepath.Ext(header.Filename))

	return a.saveConvertedUserModel(
		username,
		filepath.Base(header.Filename),
		extension,
		content,
		0,
	)
}

func (a *ServerApp) appendUserModelChunk(
	username string,
	uploadID string,
	fileName string,
	chunkIndex int,
	totalChunks int,
	chunk io.Reader,
) (*CloudModel, int, error) {
	uploadDir := a.uploadChunkDirectory(username, uploadID)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, 0, fmt.Errorf("创建分片目录失败")
	}

	partPath := a.uploadChunkPartPath(username, uploadID, chunkIndex)
	output, err := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, 0, fmt.Errorf("打开分片文件失败")
	}
	if _, err := io.Copy(output, chunk); err != nil {
		_ = output.Close()
		return nil, 0, fmt.Errorf("写入分片文件失败")
	}
	if err := output.Close(); err != nil {
		return nil, 0, fmt.Errorf("关闭分片文件失败")
	}

	progress := int(float64(chunkIndex+1) / float64(totalChunks) * 100)
	if chunkIndex < totalChunks-1 {
		return nil, progress, nil
	}

	content, err := a.readUploadedChunks(username, uploadID, totalChunks)
	if err != nil {
		return nil, progress, err
	}
	_ = os.RemoveAll(uploadDir)

	extension := normalizeExtension(filepath.Ext(fileName))
	model, err := a.saveConvertedUserModel(username, fileName, extension, content, 0)
	if err != nil {
		return nil, progress, fmt.Errorf("保存用户模型失败")
	}

	_ = a.recordModelPoolEvent(username, model, "upload")

	return &model, 100, nil
}

func (a *ServerApp) readUploadedChunks(username string, uploadID string, totalChunks int) ([]byte, error) {
	content := make([]byte, 0)
	for index := 0; index < totalChunks; index += 1 {
		partPath := a.uploadChunkPartPath(username, uploadID, index)
		partContent, err := os.ReadFile(partPath)
		if err != nil {
			return nil, fmt.Errorf("上传分片不完整，请重试")
		}

		content = append(content, partContent...)
	}

	return content, nil
}

func (a *ServerApp) saveConvertedUserModel(
	username string,
	fileName string,
	extension string,
	content []byte,
	convertedFrom int64,
) (CloudModel, error) {
	userDB, err := a.openUserDatabase(username)
	if err != nil {
		return CloudModel{}, err
	}
	defer userDB.Close()

	createdAt := time.Now().Format(time.RFC3339)

	result, err := userDB.Exec(
		`INSERT INTO models (file_name, extension, size, content, converted_from, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		filepath.Base(fileName),
		extension,
		len(content),
		content,
		convertedFrom,
		createdAt,
	)
	if err != nil {
		return CloudModel{}, err
	}

	modelID, err := result.LastInsertId()
	if err != nil {
		return CloudModel{}, err
	}

	return CloudModel{
		ID:            modelID,
		FileName:      filepath.Base(fileName),
		Extension:     extension,
		Size:          int64(len(content)),
		SizeLabel:     formatBytes(int64(len(content))),
		Previewable:   isPreviewableExtension(extension),
		NeedsConvert:  !isPreviewableExtension(extension),
		ConvertedFrom: convertedFrom,
		CreatedAt:     createdAt,
	}, nil
}

func (a *ServerApp) findUserModelContent(username string, modelID int64) (CloudModel, []byte, error) {
	userDB, err := a.openUserDatabase(username)
	if err != nil {
		return CloudModel{}, nil, err
	}
	defer userDB.Close()

	var model CloudModel
	var content []byte
	err = userDB.QueryRow(
		`SELECT id, file_name, extension, size, content, converted_from, created_at FROM models WHERE id = ?`,
		modelID,
	).Scan(
		&model.ID,
		&model.FileName,
		&model.Extension,
		&model.Size,
		&content,
		&model.ConvertedFrom,
		&model.CreatedAt,
	)
	if err != nil {
		return CloudModel{}, nil, err
	}

	model.SizeLabel = formatBytes(model.Size)
	model.Previewable = isPreviewableExtension(model.Extension)
	model.NeedsConvert = !model.Previewable

	return model, content, nil
}

func (a *ServerApp) convertUserModelWithCommand(
	username string,
	sourceModel CloudModel,
	content []byte,
	targetFormat string,
) (CloudModel, error) {
	workDir, err := os.MkdirTemp("", "meshhub-convert-*")
	if err != nil {
		return CloudModel{}, err
	}
	defer os.RemoveAll(workDir)

	inputPath := filepath.Join(workDir, sourceModel.FileName)
	outputName := buildConvertedFileName(sourceModel.FileName, targetFormat)
	outputPath := filepath.Join(workDir, outputName)

	if err := os.WriteFile(inputPath, content, 0644); err != nil {
		return CloudModel{}, err
	}

	if sourceModel.Extension == targetFormat {
		if err := os.WriteFile(outputPath, content, 0644); err != nil {
			return CloudModel{}, err
		}
	} else if a.converter != "" {
		if err := runExternalConverter(a.converter, inputPath, outputPath, targetFormat, username, sourceModel.ID); err != nil {
			return CloudModel{}, err
		}
	} else if err := runBuiltInConverter(inputPath, outputPath, sourceModel.Extension, targetFormat); err != nil {
		return CloudModel{}, err
	}

	convertedContent, err := os.ReadFile(outputPath)
	if err != nil {
		return CloudModel{}, err
	}

	return a.saveConvertedUserModel(
		username,
		outputName,
		targetFormat,
		convertedContent,
		sourceModel.ID,
	)
}

/*
|--------------------------------------------------------------------------
| 转换任务调度
|--------------------------------------------------------------------------
| 负责创建任务、查询任务、后台领取队列，并把转换结果写回用户模型库。
|--------------------------------------------------------------------------
*/
func (a *ServerApp) createConversionJob(username string, modelID int64, targetFormat string) (ConversionJob, error) {
	now := time.Now().Format(time.RFC3339)
	result, err := a.db.Exec(
		`INSERT INTO conversion_jobs (username, model_id, target_format, status, progress, message, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		username,
		modelID,
		targetFormat,
		"queued",
		0,
		"转换任务已排队。",
		now,
		now,
	)
	if err != nil {
		return ConversionJob{}, err
	}

	jobID, err := result.LastInsertId()
	if err != nil {
		return ConversionJob{}, err
	}

	return a.findConversionJob(username, jobID)
}

func (a *ServerApp) findConversionJob(username string, jobID int64) (ConversionJob, error) {
	var job ConversionJob
	err := a.db.QueryRow(
		`SELECT id, username, model_id, target_format, status, progress, message, error, result_model_id, created_at, updated_at
		 FROM conversion_jobs
		 WHERE id = ? AND username = ?`,
		jobID,
		username,
	).Scan(
		&job.ID,
		&job.Username,
		&job.ModelID,
		&job.TargetFormat,
		&job.Status,
		&job.Progress,
		&job.Message,
		&job.Error,
		&job.ResultID,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if err != nil {
		return ConversionJob{}, err
	}

	if job.ResultID > 0 {
		if model, _, modelErr := a.findUserModelContent(username, job.ResultID); modelErr == nil {
			job.ResultModel = &model
		}
	}

	return job, nil
}

func (a *ServerApp) resetRunningConversionJobs() error {
	_, err := a.db.Exec(
		`UPDATE conversion_jobs
		 SET status = 'queued', progress = 0, message = '服务重启后重新排队。', updated_at = ?
		 WHERE status = 'running'`,
		time.Now().Format(time.RFC3339),
	)

	return err
}

func (a *ServerApp) runConversionWorker() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.workerStop:
			return
		case <-ticker.C:
			a.processNextConversionJob()
		}
	}
}

func (a *ServerApp) processNextConversionJob() {
	job, err := a.takeNextConversionJob()
	if err != nil {
		return
	}

	if err := a.executeConversionJob(job); err != nil {
		_ = a.failConversionJob(job.ID, err)
	}
}

func (a *ServerApp) takeNextConversionJob() (ConversionJob, error) {
	var job ConversionJob
	err := a.db.QueryRow(
		`SELECT id, username, model_id, target_format, status, progress, message, error, result_model_id, created_at, updated_at
		 FROM conversion_jobs
		 WHERE status = 'queued'
		 ORDER BY created_at ASC
		 LIMIT 1`,
	).Scan(
		&job.ID,
		&job.Username,
		&job.ModelID,
		&job.TargetFormat,
		&job.Status,
		&job.Progress,
		&job.Message,
		&job.Error,
		&job.ResultID,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if err != nil {
		return ConversionJob{}, err
	}

	_, err = a.db.Exec(
		`UPDATE conversion_jobs
		 SET status = 'running', progress = 5, message = '后台转换任务已开始。', updated_at = ?
		 WHERE id = ? AND status = 'queued'`,
		time.Now().Format(time.RFC3339),
		job.ID,
	)
	if err != nil {
		return ConversionJob{}, err
	}

	job.Status = "running"
	job.Progress = 5
	job.Message = "后台转换任务已开始。"

	return job, nil
}

func (a *ServerApp) executeConversionJob(job ConversionJob) error {
	sourceModel, content, err := a.findUserModelContent(job.Username, job.ModelID)
	if err != nil {
		return fmt.Errorf("读取源模型失败: %w", err)
	}

	_ = a.updateConversionJobProgress(job.ID, 20, "源模型已读取，正在准备转换环境。")

	model, err := a.convertUserModelWithCommand(job.Username, sourceModel, content, job.TargetFormat)
	if err != nil {
		return err
	}

	_ = a.recordModelPoolEvent(job.Username, model, "convert")

	_, err = a.db.Exec(
		`UPDATE conversion_jobs
		 SET status = 'done', progress = 100, message = '格式转换完成。', result_model_id = ?, updated_at = ?
		 WHERE id = ?`,
		model.ID,
		time.Now().Format(time.RFC3339),
		job.ID,
	)

	return err
}

func (a *ServerApp) updateConversionJobProgress(jobID int64, progress int, message string) error {
	_, err := a.db.Exec(
		`UPDATE conversion_jobs SET progress = ?, message = ?, updated_at = ? WHERE id = ?`,
		progress,
		message,
		time.Now().Format(time.RFC3339),
		jobID,
	)

	return err
}

func (a *ServerApp) failConversionJob(jobID int64, jobErr error) error {
	_, err := a.db.Exec(
		`UPDATE conversion_jobs
		 SET status = 'failed', progress = 100, message = '格式转换失败。', error = ?, updated_at = ?
		 WHERE id = ?`,
		jobErr.Error(),
		time.Now().Format(time.RFC3339),
		jobID,
	)

	return err
}

func runExternalConverter(
	converter string,
	inputPath string,
	outputPath string,
	targetFormat string,
	username string,
	modelID int64,
) error {
	command := exec.Command(
		converter,
		inputPath,
		outputPath,
		targetFormat,
		username,
		strconv.FormatInt(modelID, 10),
	)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("外部转换器执行失败: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

func runBuiltInConverter(inputPath string, outputPath string, sourceFormat string, targetFormat string) error {
	sourceFormat = normalizeExtension(sourceFormat)
	targetFormat = normalizeExtension(targetFormat)

	if isStepExtension(sourceFormat) {
		return runFreeCADConversion(inputPath, outputPath, targetFormat)
	}

	return runAssimpConversion(inputPath, outputPath)
}

func runAssimpConversion(inputPath string, outputPath string) error {
	assimpPath, err := findExecutable("assimp")
	if err != nil {
		return errors.New("服务端未安装 assimp-utils，无法转换 STL/OBJ/FBX")
	}

	command := exec.Command(assimpPath, "export", inputPath, outputPath)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("Assimp 转换失败: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

func runFreeCADConversion(inputPath string, outputPath string, targetFormat string) error {
	freeCADPath, err := findExecutable("FreeCADCmd", "freecadcmd", "freecad")
	if err != nil {
		return errors.New("服务端未安装 FreeCAD，无法转换 STEP/STP")
	}

	targetFormat = normalizeExtension(targetFormat)
	meshPath := outputPath
	if targetFormat != "stl" {
		meshPath = filepath.Join(filepath.Dir(outputPath), "freecad-intermediate.stl")
	}

	script := `exec("""
import os
import FreeCAD
import Import
import Mesh

input_path = os.environ["INPUT_PATH"]
output_path = os.environ["OUTPUT_PATH"]

Import.open(input_path)
doc = FreeCAD.ActiveDocument
objects = [obj for obj in doc.Objects if hasattr(obj, "Shape")]
if not objects:
    raise RuntimeError("STEP/STP 文件中没有可导出的 Shape 对象")

Mesh.export(objects, output_path)
print("exported", output_path)
""")
`

	command := exec.Command(freeCADPath)
	command.Stdin = strings.NewReader(script)
	command.Env = append(os.Environ(), "INPUT_PATH="+inputPath, "OUTPUT_PATH="+meshPath)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("FreeCAD 转换失败: %w: %s", err, strings.TrimSpace(string(output)))
	}

	if _, err := os.Stat(meshPath); err != nil {
		return fmt.Errorf("FreeCAD 未生成中间网格文件: %w", err)
	}

	if targetFormat != "stl" {
		if err := runAssimpConversion(meshPath, outputPath); err != nil {
			return err
		}
	}

	return nil
}

func findExecutable(names ...string) (string, error) {
	for _, name := range names {
		path, err := exec.LookPath(name)
		if err == nil {
			return path, nil
		}
	}

	return "", errors.New("未找到可执行命令")
}

func (a *ServerApp) userDatabasePath(username string) string {
	return filepath.Join(a.userDBDir, normalizeUsername(username)+".db")
}

func (a *ServerApp) uploadChunkDirectory(username string, uploadID string) string {
	return filepath.Join(a.dataDir, "uploads", normalizeUsername(username), normalizeUploadID(uploadID))
}

func (a *ServerApp) uploadChunkPartPath(username string, uploadID string, chunkIndex int) string {
	return filepath.Join(a.uploadChunkDirectory(username, uploadID), fmt.Sprintf("%06d.part", chunkIndex))
}

/*
|--------------------------------------------------------------------------
| 访问控制中间件
|--------------------------------------------------------------------------
| 本机管理页只允许本地访问，客户端 API 使用 Bearer Token 认证。
|--------------------------------------------------------------------------
*/
func (a *ServerApp) localOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		host, _, err := net.SplitHostPort(request.RemoteAddr)
		if err != nil || !isLocalHost(host) {
			writeError(writer, http.StatusForbidden, "管理页面只允许服务器本机访问")
			return
		}

		next(writer, request)
	}
}

func (a *ServerApp) requireAuth(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}

		token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		a.sessionsMu.RLock()
		username := a.sessions[token]
		a.sessionsMu.RUnlock()

		if username == "" {
			writeError(writer, http.StatusUnauthorized, "请先登录账户")
			return
		}

		next(writer, request, username)
	}
}

func (a *ServerApp) withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")

		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}

		next(writer, request)
	}
}

/*
|--------------------------------------------------------------------------
| 共用工具函数
|--------------------------------------------------------------------------
| 放置 JSON 响应、账户名处理、令牌生成和本机地址判断等共用能力。
|--------------------------------------------------------------------------
*/
func (a *ServerApp) findPasswordHash(username string) (string, error) {
	var passwordHash string
	err := a.db.QueryRow(`SELECT password_hash FROM accounts WHERE username = ?`, username).Scan(&passwordHash)
	if err != nil {
		return "", errors.New("账户不存在")
	}

	return passwordHash, nil
}

func (a *ServerApp) recordUserPoolEvent(username string, action string) error {
	_, err := a.db.Exec(
		`INSERT INTO user_info_pool (username, action, created_at) VALUES (?, ?, ?)`,
		username,
		action,
		time.Now().Format(time.RFC3339),
	)

	return err
}

func (a *ServerApp) recordModelPoolEvent(username string, model CloudModel, action string) error {
	_, err := a.db.Exec(
		`INSERT INTO model_data_pool (username, model_id, file_name, extension, action, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		username,
		model.ID,
		model.FileName,
		model.Extension,
		action,
		time.Now().Format(time.RFC3339),
	)

	return err
}

func normalizeExtension(extension string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".")
}

func normalizeUploadID(uploadID string) string {
	cleaned := strings.Builder{}
	for _, item := range uploadID {
		if item >= 'a' && item <= 'z' || item >= 'A' && item <= 'Z' || item >= '0' && item <= '9' || item == '-' || item == '_' {
			cleaned.WriteRune(item)
		}
	}

	return cleaned.String()
}

func isPreviewableExtension(extension string) bool {
	switch normalizeExtension(extension) {
	case "stl", "obj", "fbx":
		return true
	default:
		return false
	}
}

func isSupportedTargetFormat(extension string) bool {
	switch normalizeExtension(extension) {
	case "stl", "obj", "fbx":
		return true
	default:
		return false
	}
}

func isStepExtension(extension string) bool {
	switch normalizeExtension(extension) {
	case "stp", "step":
		return true
	default:
		return false
	}
}

func buildConvertedFileName(fileName string, targetFormat string) string {
	baseName := strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
	if baseName == "" {
		baseName = "model"
	}

	return fmt.Sprintf("%s-converted.%s", baseName, normalizeExtension(targetFormat))
}

func formatBytes(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}

	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}

	return fmt.Sprintf("%.1f MB", float64(size)/1024/1024)
}

func createToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return hex.EncodeToString(bytes), nil
}

func normalizeUsername(username string) string {
	username = strings.TrimSpace(strings.ToLower(username))
	username = strings.ReplaceAll(username, "/", "")
	username = strings.ReplaceAll(username, "\\", "")

	return username
}

func isLocalHost(host string) bool {
	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

func writeJSON(writer http.ResponseWriter, status int, value interface{}) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{
		"error": message,
	})
}
