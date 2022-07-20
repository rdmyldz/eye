package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

func greet(w http.ResponseWriter, r *http.Request) {
	log.Println("log for output")
	fmt.Fprintf(w, "Hello rdm!! %s\n", time.Now())
}

func main() {
	log.Println("listening on 8080")
	http.HandleFunc("/", greet)
	http.ListenAndServe(":8080", nil)
}
