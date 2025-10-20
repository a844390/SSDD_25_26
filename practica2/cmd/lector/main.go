package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"practica2/barrier"
	"practica2/ra"
	"strconv"
	"time"
)

func close(ra *ra.RASharedDB) {
	time.Sleep(1*time.Minute + 30*time.Second)
	fmt.Println("Cerrando ms: ", ra.MS.Me)
	ra.Stop()
}

func Read(fich string) string {
	contenido, err := os.ReadFile(fich) // Lee el contenido del fichero
	if err != nil {
		log.Fatalf("Error al leer en el fichero: %v", err)
	}
	fmt.Println("Ya he leido el fichero: " + fich)
	return string(contenido)
}

func main() {
	numLinea, err := strconv.Atoi(os.Args[1])
	if err != nil || numLinea < 1 {
		fmt.Println("Número de línea inválido")
		return
	}
	barrier.HagoBarrera("../../ms/users.txt", numLinea)
	fmt.Printf("Barrera del proceso %d finalizada \n", numLinea)

	// Obtener el PID del proceso y crear un fichero con el PID
	pid := os.Getpid()
	fmt.Printf("PID del proceso %d: %d\n", numLinea, pid)
	nomFichero := strconv.Itoa(pid) + ".txt"
	fichero, err := os.Create(nomFichero)
	if err != nil {
		log.Fatalf("Error al crear el fichero: %v", err)
	}
	defer fichero.Close()

	//estructura RASharedDB
	ras := ra.New(numLinea, "../../ms/users.txt", ra.NUM_PROCESOS, "lector")
	fmt.Printf("Estructura RA del proceso %d creada\n", numLinea)
	time.Sleep(2 * time.Second) // Espera para que se activen todos los procesos
	// Lanzar el listener
	go ras.ReceiveMessages(nomFichero)
	fmt.Printf("Listener del proceso %d lanzado\n", numLinea)
	go func() {

		for {
			tiempoAleatorio := rand.Intn(5)                          // Genera un número aleatorio entre 0 y 4
			time.Sleep(time.Duration(tiempoAleatorio) * time.Second) // Duerme el proceso durante el tiempo aleatorio
			ras.PreProtocol()
			fmt.Printf("Proceso %d leyendo mensaje", numLinea)
			msj := Read(nomFichero)
			//no hace falta avisar al resto de procesos, ya que no se modifica el fichero
			ras.PostProtocol()
			fmt.Printf("Proceso %d ha escrito mensaje: %s", numLinea, msj)
			time.Sleep(10 * time.Second) // Espera antes de leer de nuevo
		}
	}()
	close(ras)
}
