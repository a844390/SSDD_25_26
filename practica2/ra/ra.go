/*
* AUTOR: Rafael Tolosana Calasanz
* ASIGNATURA: 30221 Sistemas Distribuidos del Grado en Ingeniería Informática
*			Escuela de Ingeniería y Arquitectura - Universidad de Zaragoza
* FECHA: septiembre de 2021
* FICHERO: ricart-agrawala.go
* DESCRIPCIÓN: Implementación del algoritmo de Ricart-Agrawala Generalizado en Go
 */
package ra

import (
	"fmt"
	"practica2/ms"
    "practica2/rd_wr"
	"sync"

)

const (
	NUM_LECTORES   = 1
	NUM_ESCRITORES = 1
	NUM_PROCESOS   = NUM_LECTORES + NUM_ESCRITORES
)

/*------------------------------------------------------
  DEFINICON DE MANSAJES
------------------------------------------------------*/

/*
   Mensaje Request, enviado por un proceso que desea entrar en SC
   Contiene:
   - Clock: número de secuencia (reloj lógico) del emisor
   - Pid: identificador del proceso emisor
*/
type Request struct{
    Clock   int
    Pid     int
}

/*
    Mensaje Reply: enviado en respuesta a un Request
*/
type Reply struct{}

type UpdateFile struct{
    Pid    int
    Text  string
}
/*------------------------------------------------------
    ESTRUCTURA PRINCIPAL DEL PROCESO
  ------------------------------------------------------*/
type RASharedDB struct {
    Me int                          // Identificador del proceso
    N int                           // Número total de procesos en el sistema distribuido
    OurSeqNum   int                 // Número de secuencia del proceso
    HigSeqNum   int                 // Número de secuencia más alto recibido
    OutRepCnt   int                 // Número de respuestas recibidas 
    ReqCS       bool                // Indica si el proceso quiere entrar en la sección crítica
    RepDefd     []bool              // Indica si se ha diferido el envío de un reply al proceso j 
    MS          *ms.MessageSystem   // Sistema de mensajería
    done        chan bool           // Canal para indicar que el proceso ha terminado
    chrep       chan bool           //canal para indicar que ha recibido una respuesta
    Mutex       sync.Mutex          // mutex para proteger concurrencia sobre las variables
    chareplies  chan Reply          // canal para recibir mensajes Reply
    charequests chan Request        // canal para recibir mensajes Request
    tipo       string               // tipo de proceso: lector o escritor    
}

/*------------------------------------------------------
    INICIALIZACIÓN
  ------------------------------------------------------*/

/*
    New inicializa una instancia del algoritmo Ricart-Agrawala para un proceso

    Parámetros:
    - me: id del proceso (1..N)
    - usersFile: fichero con las direcciones de los nodos (Ip:puerto)
    - n: número total de procesos
    - tipo: que clase de proceso es (lector/escritor)
    Devuelve un puntero a un objero RASharedDB inicializado
*/
func New(me int, usersFile string, tipo string) (*RASharedDB) {
    // registra los tipos de mensajes permitidos
    messageTypes := []ms.Message{Request{}, Reply{}, UpdateFile{}}
    //crea el sistema de mensajería local
    msgs := ms.New(me, usersFile, messageTypes)
    fmt.Printf("Sistema de mensajeria creado para el proceso %d\n", me)
    // inicializa la estructura de datos compartida
    ra := RASharedDB{
        OurSeqNum:          0, 
        HigSeqNum:          0, 
        OutRepCnt:          0,
        ReqCS:              false, 
        RepDefd:            make([]bool, NUM_PROCESOS), 
        MS:                 &msgs,  
        done:               make(chan bool),  
        chrep:              make(chan bool), 
        Mutex:              sync.Mutex{},
        chareplies:         make(chan Reply),
        charequests:        make(chan Request),
	    tipo:		        tipo,
	}

    go ra.handleReply()

    go ra.handleRequest()

    return &ra
}

/* ---------------------------------------------------------------------
   RECEPCIÓN Y MANEJO DE MENSAJES
   --------------------------------------------------------------------- */

/*
    Escucha continuamente los mensajes y los gestiona (handleRequest / handleReply)
*/
func (ra *RASharedDB) ReceiveMessages(ms *ms.MessageSystem, file string) {
	for {
		// Esperamos un mensaje del sistema de mensajería (bloqueante)
		raw := ms.Receive()

        fmt.Printf("Proceso %d recibe un mensaje en el RaListener\n", ra.MS.Me)
		// Determinar el tipo de mensaje recibido
		switch raw.(type) {
		    case Request:
                fmt.Printf("Proceso %d recibió mensaje request: %#v\n", ra.MS.Me, raw)
				ra.charequests <- raw.(Request) // Mensaje REQUEST recibido
			case Reply:
                fmt.Printf("Proceso %d recibió mensaje reply: %#v\n", ra.MS.Me, raw)
				ra.chareplies <- raw.(Reply) // Mensaje REPLY recibido
           case UpdateFile:
               update := raw.(UpdateFile)
               fmt.Printf("Proceso %d recibió mensaje actualizar fichero: %#v\n", ra.MS.Me, update)
                //escribir fichero
               rd_wr.WriteMessage(file, update.Text)

               newFile := rd_wr.Read(file)
               fmt.Println("Proceso" , ra.MS.Me, "actualizo fichero: ", newFile)                
			default:
				fmt.Printf("Proceso %d recibió mensaje desconocido: %#v\n", ra.MS.Me, raw)
		}
	}
}

/*------------------------------------------------------
    MANEJO DE REQUEST Y REPLY
  ------------------------------------------------------*/
/*
    Procesa un mensaje REQUEST

    Si el proceso local no quiere entrar en SC, responde inmediatamente
    Si el proceso local quiere entrar en SC, decide si se difiere o responde según los números de secuencia y los IDs de los procesos
*/
func (ra *RASharedDB) handleRequest(){
    fmt.Printf("Lanzado RecievesRequest para el proceso %d\n", ra.MS.Me)
    for {
        req := <-ra.charequests
        // varable para diferir la respuesta
        var deferReply = false

        // Actualiza el reloj más alto
        if req.Clock > ra.HigSeqNum {
            ra.HigSeqNum = req.Clock
        }

        ra.Mutex.Lock()
        // si estoy pidiendo en SC, y el proceso tiene mayor número de secuencia o si es igual, si tiene mayor pid, se difiere
        deferReply = ra.ReqCS && ((req.Clock > ra.OurSeqNum) || (req.Clock == ra.OurSeqNum && req.Pid > ra.Me))
        ra.Mutex.Unlock()

        // Si se difiere la respuesta se marca en el vector
        if deferReply {
            ra.Mutex.Lock()
            ra.RepDefd[req.Pid-1] = true
            ra.Mutex.Unlock()
        } else {
            // sin no se difiere, enviamos la respuesta
            ra.MS.Send(req.Pid, Reply{})
            fmt.Println("Enviando reply a pid: ", req.Pid, " desde el nodo ", ra.MS.Me)
        }
    }

}

/*
    Procesa un mensaje REPLY

    Cada vez que llega un reply, se decrementa el contador OutRepCnt
    Cuando este contador llegue a 0, se notifica a través del canal chrep

*/
func (ra *RASharedDB) handleReply(){
    fmt.Printf("Lanzado handleReply para el proceso %d\n", ra.MS.Me)
    for {
        <-ra.chareplies
        ra.OutRepCnt--
        if ra.OutRepCnt == 0 {
            ra.chrep <- true
        }
    }
    
}


/*------------------------------------------------------
    PROTOCOSLOS DE ENTRADA Y SALIDA DE SC
  ------------------------------------------------------*/
//Pre: Verdad
//Post: Realiza  el  PreProtocol  para el  algoritmo de
//      Ricart-Agrawala Generalizado
/*
    Preprotocolo del algoritmo RA

    Pasos:
    1. Marcar que se queire entrar en SC
    2. Incrementar el número de secuencia local
    3. Enviar REQUEST a todos los demás proceos
    4. Esperar el REPLY de todos
*/
func (ra *RASharedDB) PreProtocol(){
    fmt.Printf("Proceso %d entra a PreProtocol \n", ra.MS.Me)
    ra.Mutex.Lock()
    //indicar que se quiere entrar en SC
    ra.ReqCS = true
    //genera un número de secuencia mayor que el más alto registrado
    ra.OurSeqNum = ra.HigSeqNum + 1
    ra.Mutex.Unlock()

    //espera respuesta de todos los demás 
    ra.OutRepCnt = NUM_PROCESOS - 1

    //envío request a todos los demás
    for j := 1; j <= NUM_PROCESOS; j++ {
        if j != ra.MS.Me {
            fmt.Printf("Proceso %d envia una request a %d\n", ra.MS.Me, j)
            ra.MS.Send(j, Request{Clock: ra.OurSeqNum, Pid: ra.MS.Me})
        }
    }

    // bloqueo para esperar notificación reply
    <-ra.chrep
    fmt.Printf("Proceso %d sale de PreProtocol \n", ra.MS.Me)
}

//Pre: Verdad
//Post: Realiza  el  PostProtocol  para el  algoritmo de
//      Ricart-Agrawala Generalizado

/*
    Postprotocolo del algoritmo RA, proceso de liberación de sc

    Pasos:
    1. Marcar como fuera de SC
    2. Enviar REPLY a los procesos que han quedado diferidos 
*/
func (ra *RASharedDB) PostProtocol(){
    fmt.Printf("Proceso %d entra a PostProtocol \n", ra.MS.Me)
    ra.Mutex.Lock()
    //indicamos salida de SC
    ra.ReqCS = false
    ra.Mutex.Unlock()
    //revisa procesos diferido y manda rely
    for j := 1; j <= NUM_PROCESOS; j++ {
        if ra.RepDefd[j-1] {
            ra.RepDefd[j-1] = false
            ra.MS.Send(j, Reply{})
        }
    }
    fmt.Printf("Proceso %d sale de PostProtocol \n", ra.MS.Me)
}

/*------------------------------------------------------
    FINALIZACIÓN
  ------------------------------------------------------*/
/*
    Finaliza la ejecución del algoritmo y del sistema de mensajería 
    Cierra la gorutina recptora y libera recursos
*/
func (ra *RASharedDB) Stop(){
    ra.MS.Stop()
    ra.done <- true
}
