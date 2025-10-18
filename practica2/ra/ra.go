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
    "ms"
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
    ReqCS       boolean             // Indica si el proceso quiere entrar en la sección crítica
    RepDefd     bool[]              // Indica si se ha diferido el envío de un reply al proceso j 
    MS          *ms.MessageSystem   // Sistema de mensajería
    done        chan bool           // Canal para indicar que el proceso ha terminado
    chrep       chan bool           //canal para indicar que ha recibido una respuesta
    Mutex       sync.Mutex          // mutex para proteger concurrencia sobre las variables
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
    msgs = ms.New(me, usersFile string, messageTypes)
    // inicializa la estructura de datos compartida
    ra := RASharedDB{
        Me:                 me, 
        N:                  n, 
        OurSeqNum:          0, 
        HigSeqNum           0, 
        OutRepCnt           0,
        ReqCS:              false, 
        RepDefd             make([]bool, n+1), 
        MS:                 &msgs,  
        done:               make(chan bool),  
        chrep               make(chan bool), 
        Mutex:              &sync.Mutex{}}
    // TODO completar
    go ra.recieveMessages()

    return &ra
}

/*------------------------------------------------------
    RECEPCIÓN Y MANEJO DE MENSAJES
  ------------------------------------------------------*/
/*
    Escucha continuamente los mensajes entrantes y los despacha 
    a las funciones correspondientes (handleRequest / handleRely)
*/
func (ra *RASharedDB) recieveMessages(){}


/*------------------------------------------------------
    MANEJO DE REQUEST Y REPLY
  ------------------------------------------------------*/
/*
    Procesa un mensaje REQUEST
*/
func (ra *RASharedDB) handleRequest(){}

/*
    Procesa un mensaje REPLY
*/
func (ra *RASharedDB) handleRely(){}


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

    // esperar a recibir todos los reply (OutRepCnt == 0)
    for {
        ra.Mutex.Lock()
        if ra.OutRepCnt == 0 {
            //recibitos todos los reply, se puede entrar en SC
            ra.Mutex.Unlock()
        }
        ra.Mutex.Unlock()

        // bloqueo para esperar notificación reply
        <-ra.chrep
    }
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
    //revisa procesos diferido y manda rely
    for j := j <= ra.N; j++ {
        if ra.RepDefd[j] {
            ra.RepDefd[j] = false
            ra.MS.Send(j, Reply{})
        }
    }

    ra.Mutex.Unlock()
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
