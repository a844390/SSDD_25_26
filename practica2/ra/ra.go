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
	"sync"

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

    Devuelve un puntero a un objero RASharedDB inicializado
*/
func New(me int, usersFile string, n int) (*RASharedDB) {
    // registra los tipos de mensajes permitidos
    messageTypes := []ms.Message{Request{}, Reply{}}
    //crea el sistema de mensajería local
    msgs := ms.New(me, usersFile, messageTypes)
    // inicializa la estructura de datos compartida
    ra := RASharedDB{
        Me:                 me, 
        N:                  n, 
        OurSeqNum:          0, 
        HigSeqNum:          0, 
        OutRepCnt:          0,
        ReqCS:              false, 
        RepDefd:            make([]bool, n+1), 
        MS:                 &msgs,  
        done:               make(chan bool),  
        chrep:              make(chan bool), 
        Mutex:              sync.Mutex{},
	}
    // TODO completar

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
func (ra *RASharedDB) receiveMessages(file string) {
	for {
		// Esperamos un mensaje del sistema de mensajería (bloqueante)
		raw := ra.MS.Receive()

		// Determinar el tipo de mensaje recibido
		switch raw.(type) {
		    case Request:
                fmt.Printf("Proceso %d recibió mensaje request: %#v\n", ra.Me, raw)
				ra.charequests <- raw.(Request) // Mensaje REQUEST recibido
			case Reply:
                fmt.Printf("Proceso %d recibió mensaje reply: %#v\n", ra.Me, raw)
				ra.chareplies <- raw.(Reply) // Mensaje REPLY recibido
//            case UpdateFile:
  //              fmt.Printf("Proceso %d recibió mensaje actualizar fichero: %#v\n", ra.Me, raw)
                // escribir fichero
    //            lector.WriteMessage(file, raw.Texto)

      //          newFile := escritor.Read(file)
        //        fmt.Println("Proceso" , ra.Me, "actualizo fichero: ", file)                
			default:
				fmt.Printf("Proceso %d recibió mensaje desconocido: %#v\n", ra.Me, raw)
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
        }
    }

}

/*
    Procesa un mensaje REPLY

    Cada vez que llega un reply, se decrementa el contador OutRepCnt
    Cuando este contador llegue a 0, se notifica a través del canal chrep

*/
func (ra *RASharedDB) handleReply(){
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
    ra.Mutex.Lock()
    //indicar que se quiere entrar en SC
    ra.ReqCS = true
    ra.Mutex.Unlock()
    //genera un número de secuencia mayor que el más alto registrado
    ra.OurSeqNum = ra.HigSeqNum + 1

    //espera respuesta de todos los demás 
    ra.OutRepCnt = ra.N - 1

    //envío request a todos los demás
    for j := 1; j <= ra.N; j++ {
        if j != ra.Me {
            ra.MS.Send(j, Request{Clock: ra.OurSeqNum, Pid: ra.Me})
        }
    }

    // bloqueo para esperar notificación reply
    <-ra.chrep
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
    ra.Mutex.Lock()

    //indicamos salida de SC
    ra.ReqCS = false
    ra.Mutex.Unlock()
    //revisa procesos diferido y manda rely
    for j := 1; j <= ra.N; j++ {
        if ra.RepDefd[j-1] {
            ra.RepDefd[j-1] = false
            ra.MS.Send(j, Reply{})
        }
    }
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
