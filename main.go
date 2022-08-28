package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	log "github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

func SetupLogger(maxSize int, backups int, age int) {

	lumberjackLogger := &lumberjack.Logger{
		Filename:   "./server.log",
		MaxSize:    maxSize,
		MaxBackups: backups,
		MaxAge:     age,
		Compress:   true,
	}

	// Fork writing into two outputs
	file, err := os.OpenFile(lumberjackLogger.Filename, os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		fmt.Println("cannot create log file")
		os.Exit(1)
	}

	logFormatter := new(log.TextFormatter)
	logFormatter.TimestampFormat = time.RFC1123Z
	logFormatter.FullTimestamp = true

	log.SetFormatter(logFormatter)
	log.SetLevel(log.InfoLevel)
	log.SetOutput(file)
}

func main() {
	maxSize := flag.Int("size", 5, "max size log file in MB")
	backups := flag.Int("backups", 10, "backups is the maximum number of old log files to retain")
	age := flag.Int("age", 30, "age is the maximum number of days to retain old log files")
	certFile := flag.String("cert", "localhost.crt", "path to server certificate")
	key := flag.String("key", "localhost.key", "path to server certificate key")

	SetupLogger(*maxSize, *backups, *age)

	httpServerExitDone := &sync.WaitGroup{}

	httpServerExitDone.Add(1)
	server := startHttpServer(httpServerExitDone)
	httpServerExitDone.Add(1)
	serverSSL := startHttpsServer(httpServerExitDone, *certFile, *key)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		panic(err)
	}
	if err := serverSSL.Shutdown(ctx); err != nil {
		panic(err)
	}
	httpServerExitDone.Wait()

	fmt.Println("server ending work")
}

func startHttpsServer(wg *sync.WaitGroup, certFile string, key string) *http.Server {
	r := chi.NewRouter()
	srvSSL := &http.Server{Addr: ":8085", Handler: r}

	r.Use(middleware.Logger)

	staticFiles := http.FileServer(http.Dir("./public"))

	r.Get("/hello/{status}", hello)
	r.Post("/upload", uploadFile)
	r.Handle("/", staticFiles)

	go func() {
		defer wg.Done()

		if err := srvSSL.ListenAndServeTLS(certFile, key); err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()

	return srvSSL
}

func startHttpServer(wg *sync.WaitGroup) *http.Server {
	r := chi.NewRouter()
	srvNoSSL := &http.Server{Addr: ":8080", Handler: r}

	r.Use(middleware.Logger)

	staticFiles := http.FileServer(http.Dir("./public"))

	r.Get("/hello/{status}", hello)
	r.Post("/upload", uploadFile)
	r.Handle("/", staticFiles)

	go func() {
		defer wg.Done()

		if err := srvNoSSL.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()

	return srvNoSSL
}

func hello(w http.ResponseWriter, r *http.Request) {
	status := chi.URLParam(r, "status")
	if status == "statusnotnound" {
		log.Info("status not found")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if status == "statusbadrequest" {
		log.Info("status bad request")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if status == "statusok" {
		log.Info("status OK")
		w.WriteHeader(http.StatusOK)
		return
	}
	if status == "statusinternalservererror" {
		log.Info("status internal server error")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if status == "statusonauthoritativeinformation" {
		log.Info("status non-authoritative information")
		w.WriteHeader(http.StatusNonAuthoritativeInfo)
		return
	}

}

func uploadFile(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	file, handler, err := r.FormFile("file")
	if err != nil {
		log.Info("cannot read file : %s", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer file.Close()
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		log.Info("cannot read file : %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	fileSave, err := os.Create(handler.Filename)
	if err != nil {
		log.Info("cannot create file : %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	fileSave.Write(fileBytes)
	log.Info("file %s saved", handler.Filename)
	w.WriteHeader(http.StatusOK)
}
