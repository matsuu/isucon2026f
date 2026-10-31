package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"example.com/isureal-company/isureal"
)

func main() {
	config := isureal.ConfigFromEnv()
	opts := isureal.PresetImageOptions{
		Dir:     config.PresetImageDir,
		Workers: runtime.NumCPU(),
		Logger:  log.Default(),
	}
	flag.StringVar(&opts.Dir, "dir", opts.Dir, "preset image output directory")
	flag.IntVar(&opts.Workers, "workers", opts.Workers, "number of file writer workers")
	flag.IntVar(&opts.Limit, "limit", 0, "limit number of images to generate (0 = all)")
	flag.BoolVar(&opts.Force, "force", false, "overwrite existing preset files")
	flag.BoolVar(&opts.VisibleOnly, "visible-only", false, "only generate images currently visible to users")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := sqlx.Open("pgx", config.DatabaseDSN())
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping database: %v", err)
	}
	if err := isureal.GeneratePresetImages(ctx, db, opts); err != nil {
		log.Fatalf("generate preset images: %v", err)
	}
}
