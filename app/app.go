package main

/*
import (
	"log"
	"os"
	"net/http"
	"google.golang.org/appengine"
	"github.com/mjibson/goread/_third_party/github.com/gorilla/mux"
	app "github.com/mjibson/goread"
)

func main() {
	router := mux.NewRouter()
	app.RegisterHandlers(router)
	http.Handle("/", router)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Printf("Defaulting to port %s", port)
	}

	log.Printf("Listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
*/

import (
  "google.golang.org/appengine/v2"
  _ "github.com/mjibson/goread"
  _ "github.com/mjibson/goread/_third_party/github.com/mjibson/appstats"
)

func main() {
  appengine.Main()
}
