package main

import (
	//"errors"
	"fmt"
	//"log"
	//"math/rand"
	"net"
	"net/rpc"
	"os"
	"raft/internal/comun/check"
	"raft/internal/comun/rpctimeout"
	"raft/internal/raft"
	"strconv"
)

func main() {
	// obtener entero de indice de este nodo
	me, err := strconv.Atoi(os.Args[1])
	check.CheckError(err, "Main, mal numero entero de indice de nodo:")

	fmt.Println("Replica escucha en :", me, " de ", os.Args[2:])

	var nodos []rpctimeout.HostPort
	// Resto de argumento son los end points como strings
	// De todas la replicas-> pasarlos a HostPort
	for _, endPoint := range os.Args[2:] {
		nodos = append(nodos, rpctimeout.HostPort(endPoint))
	}

	canalAplicarOperacion := make(chan raft.AplicaOperacion, 1000)
	database := make(map[string]string)

	// Parte Servidor
	nr := raft.NuevoNodo(nodos, me, canalAplicarOperacion)
	rpc.Register(nr)

	go aplicarOperacion(database, canalAplicarOperacion)

	fmt.Println("Replica escucha en :", me, " de ", os.Args[2:])

	l, err := net.Listen("tcp", os.Args[2:][me])
	check.CheckError(err, "Main listen error:")

	for {
		rpc.Accept(l)
	}
}

func aplicarOperacion(database map[string]string, canal chan raft.AplicaOperacion) {
	for {
		operacion := <-canal
		if operacion.Operacion.Operacion == "leer" {
			operacion.Operacion.Valor = database[operacion.Operacion.Clave]
		} else if operacion.Operacion.Operacion == "escribir" {
			database[operacion.Operacion.Clave] = operacion.Operacion.Valor
			operacion.Operacion.Valor = "Operacion realizada con exito"
		}
		canal <- operacion
	}
}
