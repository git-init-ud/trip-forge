package main

import (
	"log"

	"github.com/git-init-ud/trip-forge/internal/httpapi"
)

func main() {
	log.Fatal(httpapi.Serve("rutas-crud", "127.0.0.1:8081"))
}
