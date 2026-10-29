// Caminho: catraca/main.go

package main

import (
	"context"
	"database/sql"
	"embed"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"pdv/internal/applog"
	"pdv/internal/db"
	"pdv/internal/service"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var (
		dataDir, dbPath, notice string
		startupErr              error
	)

	dataDir, startupErr = db.DataDir()
	if startupErr == nil {
		if l, closer, err := applog.Open(dataDir); err == nil {
			logger = l
			defer closer.Close()
		}
		dbPath = filepath.Join(dataDir, db.FileName)
		// Uma restauração agendada é aplicada antes de abrir o banco.
		if out, err := service.ApplyPendingRestore(ctx, dataDir, time.Now()); err != nil {
			logger.Error("aplicar restauração", "erro", err)
			notice = "A restauração agendada não pôde ser aplicada; o banco anterior foi mantido."
		} else {
			notice = out.Message
		}
	}
	var conn *sql.DB
	if startupErr == nil {
		if conn, startupErr = db.Open(ctx, dbPath); startupErr != nil {
			conn = nil
		}
	}
	if startupErr != nil {
		logger.Error("inicializar banco", "erro", startupErr)
	} else {
		logger.Info("banco aberto", "versao_esquema", db.LatestVersion())
	}

	app := NewApp(logger, conn, dataDir, dbPath, startupErr, notice)
	err := wails.Run(&options.App{
		Title:     "PDV Academia",
		// Uma cópia do programa por PC: abrir de novo só traz a janela para frente.
		// Também garante um só dono para a porta do modo online do iDFace.
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "8e3f6a21-9c4d-4b7e-a512-6d0c2f9b7e48",
			OnSecondInstanceLaunch: app.onSecondInstance,
		},
		WindowStartState: options.Maximised,
		Width:     1280,
		Height:    800,
		MinWidth:  1024,
		MinHeight: 680,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 245, G: 246, B: 248, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		Windows:          &windows.Options{},
	})
	if err != nil {
		logger.Error("executar aplicação", "erro", err)
		os.Exit(1)
	}
}
