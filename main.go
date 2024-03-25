package main

import (
	"flag"
	"fmt"
	"github.com/MrMelon54/exit-reload"
	"github.com/julienschmidt/httprouter"
	"github.com/robfig/cron/v3"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var codeExts = []string{"js", "ts", "go", "py", "java", "html", "svelte", "css", "scss"}

func main() {
	usernameFlag := flag.String("user", "", "username to fetch github repos from")
	listenFlag := flag.String("listen", ":8080", "address:port to listen on")
	basePath := flag.String("base", ".", "base path to store data")
	flag.Parse()

	if *usernameFlag == "" {
		log.Fatal("Invalid -user flag")
	}

	wd := *basePath
	indexStorage := filepath.Join(wd, "indexes")

	err := os.MkdirAll(indexStorage, os.ModePerm)
	if err != nil {
		log.Fatal("Failed to create index storage directory:", err)
		return
	}

	db, err := InitDB(filepath.Join(wd, "random-source.sqlite.db"))
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	repoService := NewRepoService(db, *usernameFlag, http.DefaultClient)
	indexService := NewIndexingService(db, indexStorage)

	cronMgr := cron.New()
	cronMgr.Start()
	_, _ = cronMgr.AddFunc("@weekly", func() {
		repoService.Run()
		indexService.Run()
	})

	log.Println("Running initial repository sync")
	n := time.Now()
	repoService.Run()
	nSince := time.Since(n)
	log.Println("Finished initial repository sync in", nSince)

	// call index service in case changes were made
	go indexService.Run()

	r := httprouter.New()
	r.GET("/", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		index, err := indexService.db.RandomIndexedFile(req.Context(), 50)
		if err != nil {
			http.Error(rw, "500 Internal Server Error", http.StatusInternalServerError)
			return
		}
		open, err := os.Open(filepath.Join(indexStorage, fmt.Sprintf("%d.bin", index.ID)))
		if err != nil {
			http.Error(rw, "500 Internal Server Error", http.StatusInternalServerError)
			return
		}
		_, _ = io.Copy(rw, open)
	})
	httpSrv := http.Server{
		Addr:              *listenFlag,
		Handler:           r,
		ReadTimeout:       time.Minute,
		ReadHeaderTimeout: time.Minute,
		WriteTimeout:      time.Minute,
		IdleTimeout:       time.Minute,
		MaxHeaderBytes:    1024,
	}
	go func() {
		_ = httpSrv.ListenAndServe()
	}()

	exit_reload.ExitReload("RandomSource", func() {}, func() {
		_ = httpSrv.Close()
		<-cronMgr.Stop().Done()
	})
}
