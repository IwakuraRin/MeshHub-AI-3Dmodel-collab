/*
|--------------------------------------------------------------------------
| Wails 桌面端入口
|--------------------------------------------------------------------------
| 嵌入已构建的前端资源，并启动原生应用窗口。
|--------------------------------------------------------------------------
*/
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend_dist
var assets embed.FS

/*
|--------------------------------------------------------------------------
| 桌面应用启动流程
|--------------------------------------------------------------------------
| 创建后端服务，配置窗口参数，嵌入前端资源，并启动 Wails 应用。
|--------------------------------------------------------------------------
*/
func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "MeshHub",
		Width:  1200,
		Height: 760,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.Startup,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
