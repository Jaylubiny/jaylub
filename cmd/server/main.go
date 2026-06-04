package main

import (
	"fmt"
	"jaylub/internal/auth"
	"log"
	"net/http"

	router "jaylub/internal/routers"
)

type server struct {
	addr    string
	handler http.Handler
}

func main() {
	authService, err := auth.New("internal/database/users.db")
	if err != nil {
		log.Fatal(err)
	}
	defer authService.Close()

	servers := []server{
		{":8080", router.Basic(authService)},
		{":8090", router.Company(authService)},
	}

	for _, srv := range servers[:len(servers)-1] {
		go startServer(srv)
	}

	startServer(servers[len(servers)-1])
}

func startServer(srv server) {
	fmt.Println("Server running on " + srv.addr)
	if err := http.ListenAndServe(srv.addr, srv.handler); err != nil {
		log.Println(err)
	}
}
