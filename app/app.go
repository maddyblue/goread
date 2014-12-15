package app

import (
	"github.com/gorilla/mux"
	"net/http"

	app "github.com/mjibson/goread"
)

func init() {
	router := mux.NewRouter()
	app.RegisterHandlers(router)
	http.Handle("/", router)
}
