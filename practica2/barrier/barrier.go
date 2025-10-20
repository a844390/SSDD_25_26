package barrier

// ================================================================
// barrier.go
// Autores: Pedro Chaves y Beatriz Fetita
// ---------------------------------------------------------------
// Implementación básica de una BARRERA DISTRIBUIDA en Go.
// Cada proceso se ejecuta por separado, leyendo un archivo
// con las direcciones (IP:puerto) de todos los procesos del sistema.
//
// Cuando todos los procesos alcanzan la barrera, pueden continuar.
//
// Ejecución:
//    go run barrier.go endpoints.txt <num_linea>
//
// Ejemplo de endpoints.txt:
//    localhost:8001
//    localhost:8002
//    localhost:8003
//
// Ejecutar en tres terminales distintas:
//    go run barrier.go endpoints.txt 1
//    go run barrier.go endpoints.txt 2
//    go run barrier.go endpoints.txt 3
// ================================================================

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

// ================================================================
// Función: readEndpoints
// ---------------------------------------------------------------
// Lee desde un archivo una lista de endpoints (uno por línea),
// por ejemplo: "localhost:8001", "localhost:8002", etc.
// Devuelve un slice con todas las direcciones IP:puerto.
// ================================================================
func readEndpoints(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var endpoints []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			endpoints = append(endpoints, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return endpoints, nil
}

// ================================================================
// Función: handleConnection
// ---------------------------------------------------------------
// Esta función se ejecuta por cada conexión entrante.
// Lee el identificador del proceso remoto y marca en el mapa
// "received" que ese proceso alcanzó la barrera.
//
// Cuando todos los procesos (n-1) han llegado, envía 'true'
// por el canal "barrierChan" para indicar que se puede continuar.
// ================================================================
func handleConnection(conn net.Conn, barrierChan chan<- bool, quitChannel chan<- bool, received *map[string]bool, mu *sync.Mutex, n int) {
	defer conn.Close()
	buf := make([]byte, 1024)
	_, err := conn.Read(buf)
	if err != nil {
		fmt.Println("Error reading from connection:", err)
		return
	}
	msg := string(buf)
	mu.Lock()
	(*received)[msg] = true
	fmt.Println("Process ", msg, " reached the barrier (",len(*received),"/",n-1,") \n")
	if len(*received) == n-1 {
		barrierChan <- true
		quitChannel <- true
	}
	mu.Unlock()
}

// ================================================================
// Función: getEndpoints
// ---------------------------------------------------------------
// Lee los argumentos del programa:
//   - Nombre del archivo con los endpoints.
//   - Número de línea que identifica a este proceso.
//
// Devuelve:
//   - Lista de endpoints
//   - Línea (índice) correspondiente al proceso actual
//   - Error si algo falla
//
// ================================================================
func getEndpoints() ([]string, int, error) {
	if len(os.Args) != 3 {
		return nil, 0, errors.New("Usage: go run barrier.g <endpoints file> <line number>")
	}

	endpointsFile := os.Args[1]
	/* Hacemos esta declaración porque más abajo cuando se asigna
	   valor en "endpoints, err = readEndpoints(endpointsFile)"
	   err ya existe y por tanto se tiene que usar "=". En go
	   cuando se usa ":=" es para crear una variable y asignarle
	   valor.*/
	var endpoints []string // Por qué esta declaración ?
	lineNumber, err := strconv.Atoi(os.Args[2])
	if err != nil || lineNumber < 1 {
		fmt.Println("Invalid line number")
	} else if endpoints, err = readEndpoints(endpointsFile); err != nil {
		fmt.Println("Error reading endpoints:", err)
	} else if lineNumber > len(endpoints) {
		fmt.Printf("Line number %d out of range\n", lineNumber)
		err = errors.New("Line number out of range")
	}

	return endpoints, lineNumber, err
}

// ================================================================
// Función: acceptAndHandleConnections
// ---------------------------------------------------------------
// Acepta conexiones entrantes y lanza una goroutine para cada una.
// Cuando recibe la señal en "quitChannel", detiene el listener.
// ================================================================
func acceptAndHandleConnections(listener net.Listener, quitChannel chan bool,
	barrierChan chan bool, receivedMap *map[string]bool, mu *sync.Mutex, n int) {
	for {
		select {
		case <-quitChannel:
			fmt.Println("Stopping the listener...")
			break
		default:
			conn, err := listener.Accept()
			if err != nil {
				fmt.Println("Error accepting connection:", err)
				continue
			}
			go handleConnection(conn, barrierChan, quitChannel, receivedMap, mu, n)
		}
	}
}

// ================================================================
// Función: notifyOtherDistributedProcesses
// ---------------------------------------------------------------
// Cuando el proceso actual llega a la barrera, notifica a todos
// los demás procesos estableciendo una conexión TCP con cada uno
// y enviando su número de línea.
// ================================================================
func notifyOtherDistributedProcesses(endPoints []string, lineNumber int) {
	for i, ep := range endPoints {
		if i+1 != lineNumber {
			go func(ep string) {
				for {
					conn, err := net.Dial("tcp", ep)
					if err != nil {
						fmt.Println("Error connecting to", ep, ":", err)
						time.Sleep(1 * time.Second)
						continue
					}
					_, err = conn.Write([]byte(strconv.Itoa(lineNumber)))
					if err != nil {
						fmt.Println("Error sending message:", err)
						conn.Close()
						continue
					}
					conn.Close()
					break
				}
			}(ep)
		}
	}
}

// ================================================================
// Función principal (main)
// ---------------------------------------------------------------
// 1. Obtiene la configuración del proceso (endpoint local y globales)
// 2. Inicia un listener para recibir notificaciones de otros procesos
// 3. Envía notificaciones a los demás procesos
// 4. Espera hasta que todos lleguen a la barrera
// ================================================================
func main() {
	/*	var listener net.Listener

			if len(os.Args) != 3 {
				fmt.Println("Usage: go run main.go <endpoints_file> <line_number>")
			} else if endPoints, lineNumber, err := getEndpoints(); err != nil {
		        	// Get the endpoint for current process
		        	localEndpoint := endPoints[lineNumber-1]
			        if listener, err = net.Listen("tcp", localEndpoint); err != nil {
				    fmt.Println("Error creating listener:", err)
				} else {
					fmt.Println("========================================")
					fmt.Println("Process:", lineNumber)
					fmt.Println("Listening on:", localEndpoint)
					fmt.Println("Waiting for other processes to reach the barrier...")
					fmt.Println("========================================")
				}
			}
	*/ // Sintaxis confusa
	endPoints, lineNumber, err := getEndpoints()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	localEndpoint := endPoints[lineNumber-1]
	listener, err := net.Listen("tcp", localEndpoint)
	if err != nil {
		fmt.Println("Error creating listener:", err)
		return
	}
	fmt.Println("========================================")
	fmt.Println("Process:", lineNumber)
	fmt.Println("Listening on:", localEndpoint)
	fmt.Println("Waiting for other processes to reach the barrier...")
	fmt.Println("========================================")

	// Barrier synchronization
	var mu sync.Mutex
	quitChannel := make(chan bool)
	receivedMap := make(map[string]bool)
	barrierChan := make(chan bool)

	go acceptAndHandleConnections(listener, quitChannel, barrierChan, &receivedMap, &mu, len(endPoints))

	notifyOtherDistributedProcesses(endPoints, lineNumber)

	fmt.Println("Waiting for all the processes to reach the barrier")
	<-barrierChan
	fmt.Println("Every process reached the barrier. Resuming execution...")

	listener.Close()
	fmt.Println("Listener closed")
}
