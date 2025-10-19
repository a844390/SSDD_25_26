package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"practica2/ra"
	"strconv"
	"strings"
	"time"
)

func getAddress(path string) (address []string) {
	file, err := os.Open(path)
	checkError(err)
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		address = append(address, scanner.Text())
	}
	return address
}

func launchProcess(address string, actor string, numLinea string) {
	ip := strings.Split(address, ":")
	if actor == "reader" {
		fmt.Println("Ejecuto lector");
		comando := exec.Command("ssh", "-n", "a840269@"+ip[0], "export PATH=$PATH:/usr/local/go/bin/;",  "cd /misc/alumnos/sd/sd2526/a840269/practica2/cmd/lector;", "go run main.go",  numLinea)
		comando.Stdin = os.Stdin
		comando.Stderr = os.Stderr
		comando.Stdout = os.Stdout
		err := comando.Run()
		if err != nil {
			fmt.Println("Error en el comando")
		}
	} else if actor == "writer" {
		fmt.Println("Ejecutor escritor");
		comando := exec.Command("ssh", "-n", "a840269@"+ip[0],  "export PATH=$PATH:/usr/local/go/bin/;",  "cd /misc/alumnos/sd/sd2526/a840269/practica2/cmd/escritor;", "go run main.go",  numLinea)
		comando.Stdin = os.Stdin
		comando.Stderr = os.Stderr
		comando.Stdout = os.Stdout
		err := comando.Run()
		if err != nil {
			fmt.Println("Error en el comando")
		}
	}
}

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}

func main() {
	adress := getAddress("./ms/users.txt")
	readers := 1
	for ; readers <= ra.NUM_LECTORES; readers++ {
		go launchProcess(adress[readers-1], "reader", strconv.Itoa(readers))
		time.Sleep(2 * time.Second)
	}
	for writers := readers; writers <= ra.NUM_PROCESOS; writers++ {
		go launchProcess(adress[writers-1], "writer", strconv.Itoa(writers))
		time.Sleep(2 * time.Second)
	}
	for{}

}
