package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"practica2/barrier"
	"practica2/ra" //Fucniones de lectura y escritura de ficheros
	"strconv"
	"time"
)

func close(ra *ra.RASharedDB) {
	time.Sleep(1*time.Minute + 30*time.Second)
	fmt.Println("Cerrando ms: ", ra.Ms.Me)
	ra.Stop()
}

func WriteMessage(fich string, msj string) {
	fichero, err := os.OpenFile(fich, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666) // Abre el fichero en modo escritura
	if err != nil {
		log.Fatalf("Error al abrir el fichero: %v", err)
	}
	defer fichero.Close()
	_, err = fichero.WriteString(msj) // Escribe el texto en el fichero
	if err != nil {
		log.Fatalf("Error al escribir en el fichero: %v", err)
	}
	fmt.Println("Ya he escrito en el fichero: " + fich)
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
	ras := ra.New(numLinea, "../../ms/users.txt", "escritor")
	fmt.Printf("Estructura RA del proceso %d creada\n", numLinea)
	time.Sleep(2 * time.Second) // Espera para que se activen todos los procesos
	// Lanzar el listener
	go ras.RaListener(ras.Ms, nomFichero)
	fmt.Printf("Listener del proceso %d lanzado\n", numLinea)
	go func() {
		msj := "Escribe: " + strconv.Itoa(pid) + "\n"

		for {
			tiempoAleatorio := rand.Intn(5)                          // Genera un número aleatorio entre 0 y 4
			time.Sleep(time.Duration(tiempoAleatorio) * time.Second) // Duerme el proceso durante el tiempo aleatorio
			ras.PreProtocol()
			fmt.Printf("Proceso %d escribiendo mensaje: %s", numLinea, msj)
			WriteMessage(nomFichero, msj)
			for i := 1; i <= ra.NUM_PROCESOS; i++ {
				//Si no soy yo envio aviso de que he escrito
				if i != ras.Ms.Me {
					ras.Ms.Send(i, ra.ActualizaFichero{
						Pid:      i,
						Escritor: ras.Ms.Me,
					})
				}
			}
			ras.PostProtocol()
		}
	}()
	close(ras)

}
