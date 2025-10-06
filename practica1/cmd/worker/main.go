/*
* AUTOR: Pedro Chaves Muniesa, Beatriz Emanuela Fetita
* ASIGNATURA: 30221 Sistemas Distribuidos del Grado en Ingeniería Informática
*			Escuela de Ingeniería y Arquitectura - Universidad de Zaragoza
* FECHA: octubre de 2025
* FICHERO: worker.go
* DESCRIPCIÓN: contenido del fichero worker
 */
package main

import (
	"encoding/gob"
	"fmt"
	"log"
	"net"
	"os"
	"practica1/com"
)

// PRE: verdad = !foundDivisor
// POST: IsPrime devuelve verdad si n es primo y falso en caso contrario
func isPrime(n int) (foundDivisor bool) {
	foundDivisor = false
	for i := 2; (i < n) && !foundDivisor; i++ {
		foundDivisor = (n%i == 0)
	}
	return !foundDivisor
}

// PRE: interval.A < interval.B
// POST: FindPrimes devuelve todos los números primos comprendidos en el
//
//	intervalo [interval.A, interval.B]
func findPrimes(interval com.TPInterval) (primes []int) {
	for i := interval.Min; i <= interval.Max; i++ {
		if isPrime(i) {
			primes = append(primes, i)
		}
	}
	return primes
}

func processRequest(conn net.Conn) {
	var request com.Request
	decoder := gob.NewDecoder(conn)
	err := decoder.Decode(&request)
	com.CheckError(err)
	primes := findPrimes(request.Interval)
	reply := com.Reply{Id: request.Id, Primes: primes}
	encoder := gob.NewEncoder(conn)
	encoder.Encode(&reply)
}

func main() {
	fmt.Println("Entra en el worker")
	args := os.Args
	if len(args) != 3 {
		log.Println("Error: endpoint missing: go run server.go ip:port(worker) ip:port(master)")
		os.Exit(1)
	}
	endpoint := args[1]
	endMaster := args[2]

	// Indicador de conexión correcta con el master
	conn, err := net.Dial("tcp", endMaster)
	com.CheckError(err)
	encoder := gob.NewEncoder(conn)
	ready := "ready"
	err = encoder.Encode(ready)
	com.CheckError(err)
	conn.Close()

	listener, err := net.Listen("tcp", endpoint)
	com.CheckError(err)

	// log.SetFlags(log.Lshortfile | log.Lmicroseconds)

	// log.Println("***** Listening for new connection in endpoint ", endpoint)
	for {
		conn, err := listener.Accept()
		defer conn.Close()
		com.CheckError(err)
		go processRequest(conn)
	}
}
