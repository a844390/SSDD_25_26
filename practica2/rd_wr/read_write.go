package rd_wr

import (
	"fmt"
	"log"
	"os"
)

func Read(fich string) string {
	contenido, err := os.ReadFile(fich) // Lee el contenido del fichero
	if err != nil {
		log.Fatalf("Error al leer en el fichero: %v", err)
	}
	fmt.Println("Ya he leido el fichero: " + fich)
	return string(contenido)
}

func WriteMessage(fich string, msj string) string {
	fichero, err := os.OpenFile(fich, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666) // Abre el fichero en modo escritura
	fmt.Printf("WriteMessage escribir en fichero : %v\n" , fichero)
	if err != nil {
		log.Fatalf("Error al abrir el fichero: %v", err)
	}
	defer fichero.Close()
	_, err = fichero.WriteString(msj) // Escribe el texto en el fichero
	if err != nil {
		log.Fatalf("Error al escribir en el fichero: %v", err)
	}
	fmt.Println("Ya he escrito en el fichero: " + fich)

	return msj
}

