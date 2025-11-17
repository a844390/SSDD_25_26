// =======================================
// IMPLEMENTACIÓN DE RAFT EN GO
// AUTORES: PEDRO CHAVES Y BEATRIZ FETITA
// =======================================

package raft

//
// API
// ===
// Este es el API que vuestra implementación debe exportar
//
// nodoRaft = NuevoNodo(...)
//   Crear un nuevo servidor del grupo de elección.
//
// nodoRaft.Para()
//   Solicitar la parado de un servidor
//
// nodo.ObtenerEstado() (yo, mandato, esLider)
//   Solicitar a un nodo de elección por "yo", su mandato en curso,
//   y si piensa que es el msmo el lider
//
// nodoRaft.SometerOperacion(operacion interface()) (indice, mandato, esLider)

// type AplicaOperacion


import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"math/rand"
	//"crypto/rand"
	"sync"
	"time"
	//"net/rpc"

	"raft/internal/comun/rpctimeout"
)


const (
	// Constante para fijar valor entero no inicializado
	IntNOINICIALIZADO = -1

	//  false deshabilita por completo los logs de depuracion
	// Aseguraros de poner kEnableDebugLogs a false antes de la entrega
	kEnableDebugLogs = true

	// Poner a true para logear a stdout en lugar de a fichero
	kLogToStdout = false

	// Cambiar esto para salida de logs en un directorio diferente
	kLogOutputDir = "./logs_raft/"
)

// Tipo de operación para el log
type TipoOperacion struct {
	Operacion string  // La operaciones posibles son "leer" y "escribir"
	Clave string
	Valor string    // en el caso de la lectura Valor = ""
}


// A medida que el nodo Raft conoce las operaciones de las  entradas de registro
// comprometidas, envía un AplicaOperacion, con cada una de ellas, al canal
// "canalAplicar" (funcion NuevoNodo) de la maquina de estados 
type AplicaOperacion struct {
	Indice int  // en la entrada de registro
	Operacion TipoOperacion
}

// entrada en el log de Raft
// cada nodo de Raft mantiene un array de LogEntry que contiene las operaciones
// pendientes o comprometidas que deben aplicarse a la maquina de estados replicada
type LogEntry struct {
	Index     int				// indice de esta entrada dentro del log (iniciada en 1)
	Term      int				// termino en el que el lider recibio esta entrada (para detectar inconsistencias en el log)
	Operation TipoOperacion		// comando que se debe aplicar a la maquina de estados
}

// Estados posibles de un nodo en Raft
type EstadoNodo int
const (
	Seguidor EstadoNodo = iota
	Candidato
	Lider
)

// Tipo de dato Go que representa un solo nodo (réplica) de raft
//
type NodoRaft struct {
	Mux   sync.Mutex       			// Mutex para proteger acceso a estado compartido

	// Host:Port de todos los nodos (réplicas) Raft, en mismo orden
	Nodos []rpctimeout.HostPort		//Lista de todos los nodos raft
	Yo    int           			// indice de este nodos en campo array "nodos"
	IdLider int						// id del lider actual conocido
	// Utilización opcional de este logger para depuración
	// Cada nodo Raft tiene su propio registro de trazas (logs)
	Logger *log.Logger

	// Vuestros datos aqui.
	State EstadoNodo			//estado actual: lider, candidato, seguidor
	// Estado persistente en todos los servers
	// Guardado en almacenamiento estable antes de responder a un RPC
	CurrentTerm int				// ultimo termino que ha visto el nodo (inicializado a 0, aumenta monotónicamente)
	VotedFor    int				// id del candidato al que este nodo voto en el trmino actual (-1 si no ha votado)
	Log         []LogEntry		// log de entradas: cada entrada contiene un comando y el termino en el que fue recibido por el lider

	// Estado volatil en todos los servidores
	// (no se guarda en disco, se reinicia al arrancar)
	CommitIndex int				// indice de la entrada más alta conocida comprometida
	LastApplied int				// indice de la entrada más alta aplicada a la máquina de estados

	LastTermApplied	int			// término de la última entrada aplicada a la máquina de estados
	// Estado volátil en líderes (reiniciado tras cada elección)
	NextIndex []int				// para cada servidor: índice de la siguiente entrada de log que debe enviarse al ese seguidor
	MatchIndex []int			// para cada servidor: índice de la entrada más alta replicada en ese seguidor

	opComprometida chan string	// canal para notificar que una operación se ha comprometido
	receivedHeartBeat chan bool	// canal para notificar la recepción de un latido (heartbeat)

	termUpdated chan bool		// canal para notificar que el término ha sido actualizado
	serSeguidor chan bool		// canal para notificar el cambio a estado de seguidor
	serLider chan bool			// canal para notificar el cambio a estado de líder

	EnVotacion bool			// indica si el nodo está en proceso de votación
	AplicaOperacion chan AplicaOperacion //Aplicacion de operaciones a la máquina de estados
	ResultOperacion chan string          //Canal para devolver el resultado de una operación

	numVotos      int
	numRespuestas int
}



// Creacion de un nuevo nodo de eleccion
//
// Tabla de <Direccion IP:puerto> de cada nodo incluido a si mismo.
//
// <Direccion IP:puerto> de este nodo esta en nodos[yo]
//
// Todos los arrays nodos[] de los nodos tienen el mismo orden

// canalAplicar es un canal donde, en la practica 5, se recogerán las
// operaciones a aplicar a la máquina de estados. Se puede asumir que
// este canal se consumira de forma continúa.
//
// NuevoNodo() debe devolver resultado rápido, por lo que se deberían
// poner en marcha Gorutinas para trabajos de larga duracion
func NuevoNodo(nodos []rpctimeout.HostPort, yo int, 
						canalAplicarOperacion chan AplicaOperacion) *NodoRaft {
	nr := &NodoRaft{}

	// inicializacion de variables
	nr.Nodos = nodos		//lista de todos los nodos
	nr.Yo = yo				//indice en la lista de nodos
	nr.IdLider = -1			//no hay lider conocido

	nr.State = Seguidor		// estado inicial de seguidor

	nr.CurrentTerm = 0		//termino inicial
	nr.VotedFor = -1		//no se ha votado en el termino actual

	nr.CommitIndex = -1 // Inicialmente no hay entradas comprometidas
	nr.LastApplied = -1 // Inicialmente no hay entradas aplicadas

	nr.NextIndex = make([]int, len(nodos))  // Inicialmente no hay entradas para enviar
	nr.MatchIndex = make([]int, len(nodos)) // Inicialmente no hay entradas replicadas

	nr.opComprometida = make(chan string)
	nr.receivedHeartBeat = make(chan bool)

	nr.termUpdated = make(chan bool, len(nodos))
	nr.serSeguidor = make(chan bool)
	nr.serLider = make(chan bool)
	nr.EnVotacion = false		//actualmente en proceso de votación
	nr.AplicaOperacion = canalAplicarOperacion
	nr.ResultOperacion = make(chan string)

	if kEnableDebugLogs {
		nombreNodo := nodos[yo].Host() + "_" + nodos[yo].Port()
		logPrefix := fmt.Sprintf("%s", nombreNodo)
		
		fmt.Println("LogPrefix: ", logPrefix)

		if kLogToStdout {
			nr.Logger = log.New(os.Stdout, nombreNodo + " -->> ",
								log.Lmicroseconds|log.Lshortfile)
		} else {
			err := os.MkdirAll(kLogOutputDir, os.ModePerm)
			if err != nil {
				panic(err.Error())
			}
			logOutputFile, err := os.OpenFile(fmt.Sprintf("%s/%s.txt",
			  kLogOutputDir, logPrefix), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				panic(err.Error())
			}
			nr.Logger = log.New(logOutputFile, 
						   logPrefix + " -> ", log.Lmicroseconds|log.Lshortfile)
		}
		nr.Logger.Println("logger initialized")
	} else {
		nr.Logger = log.New(ioutil.Discard, "", 0)
	}

	// Añadir codigo de inicialización
	fmt.Printf("NodoRaft %d creado\n", yo)
	go CicloDeVida(nr)

	return nr
}

// Funcion para obtener un timeout aleatorio de entre a y a+b
func obtenerTimeoutAleatorio(a int, b int) time.Duration {
	return time.Duration(a+rand.Intn(b)) * time.Millisecond
}

// CicloDeVida gestiona el comportamiento principal de un nodo Raft.
//
// Esta función se ejecuta de manera concurrente para cada nodo.
// Implementa el bucle infinito que controla la máquina de estados de Raft:
// cada nodo puede estar en uno de tres estados: Seguidor, Candidato o Líder.
func CicloDeVida(nr *NodoRaft) {
	time.Sleep(10000 * time.Millisecond)
	for {
		fmt.Printf("NodoRaft %d CicloDeVida\n", nr.Yo)

		//Antes de gestionar el estado se buscan operaciones para aplicar
		if nr.CommitIndex > nr.LastApplied {
			//si el número de entradas comprometidas es mayor a las aplicadas
			nr.LastApplied++	//incrementa las aplicadas
			//crea la operación con índice la última en aplicar y la operación correspondiente 
			operacion := AplicaOperacion{nr.LastApplied, nr.Log[nr.LastApplied].Operation}
			//la manda por el canal
			nr.AplicaOperacion <- operacion
		}

		switch nr.State {
		case Seguidor:
			fmt.Println("Soy follower")
			//Establecemos un timeout aleatorio entre 1 y 2 segundos
			//la base para iniciar una elección si no se recibe un heartbeat
			timeout := time.NewTimer(obtenerTimeoutAleatorio(1000,1000)) 
			select {
			case <-timeout.C:
				//expita el timeout
				fmt.Println("Timeout salta, paso a ser candidato")
				nr.State = Candidato	//cambio de estado a candidato
				nr.IdLider = -1 		//No hay lider concido
			case <-nr.receivedHeartBeat: //Si recibo un heartbeat es que hay un lider activo
				fmt.Println("Heartbeat recibido")
				nr.State = Seguidor		//se mantiene como seguidor
			}
		case Candidato:
			fmt.Println("Soy Candidato")
			nr.CurrentTerm++		//incrementa el termino actual antes de inciar una nueva votacion
			nr.VotedFor = nr.Yo		//se vota a si mismo
			nr.numVotos = 1			//cuenta su propio voto
			//Antes de iniciar el proceso de votación actualiza su máquina de estados para que refleje todo lo comprometido
			if nr.CommitIndex > nr.LastApplied {
				nr.LastApplied++
				operacion := AplicaOperacion{nr.LastApplied, nr.Log[nr.LastApplied].Operation}
				nr.AplicaOperacion <- operacion
			}
			go nr.iniciarProcesoVotacion()
			//timer para la duracion máxima de la votación
			timer := time.NewTimer(obtenerTimeoutAleatorio(100,400))
			select {
			case <-nr.receivedHeartBeat:
				//se recive latido, hay un lider activo y se vuelve al estado de seguidor
				nr.State = Seguidor

			case <-timer.C:
				//acaba el tiempo de eleccion, se inicia una nueva
				nr.State = Candidato

			case <-nr.serSeguidor:
				//recepcion de una señal para volver a ser seguidor
				nr.State = Seguidor

			case <-nr.serLider:
				//recepción de señal para ser lider
				for i := 0; i < len(nr.Nodos); i++ {
					if i != nr.Yo {
						nr.NextIndex[i] = len(nr.Log)//para saber desde donde se van a enviar las entradas
						nr.MatchIndex[i] = -1 //inicializa los indices replicados
					}
				}
				nr.State = Lider
			}
		case Lider:
			fmt.Println("Soy Lider")
			nr.IdLider = nr.Yo		//lider actual
			//Envía heartbeats (AppendEntries vacio) a todos los seguidores para que inicien votación
			sendAppendEntries(nr)
			// timer para la frecuencia de envio de latido
			timer := time.NewTimer(100 * time.Millisecond)

			select {
			case <-nr.serSeguidor:
				//señal para volver a ser seguidor (por la llegada de un termino mayor)
				nr.State = Seguidor
			case <-timer.C:
				//expira el timer, sigue siendo líder y mandando latido
				if nr.CommitIndex > nr.LastApplied {
					//mantiene la máquina de estados actualizada
					nr.LastApplied++
					operacion := AplicaOperacion{nr.LastApplied, nr.Log[nr.LastApplied].Operation}
					nr.AplicaOperacion <- operacion

					//responde al cliente con la operación comprometida
					operacion = <-nr.AplicaOperacion
					nr.ResultOperacion <- operacion.Operacion.Valor
				}
				nr.State = Lider
			}

		}
	}

}

// Metodo Para() utilizado cuando no se necesita mas al nodo
//
// Quizas interesante desactivar la salida de depuracion
// de este nodo
//
func (nr *NodoRaft) para() {
	go func() {time.Sleep(5 * time.Millisecond); os.Exit(0) } ()
}

//Líder envía una operación concreta
func (nr *NodoRaft) enviarOperacion(nodo int, args *ArgAppendEntries, reply *Results) bool {
	//AppendEntries al nodo
	err := nr.Nodos[nodo].CallTimeout("NodoRaft.AppendEntries", args, reply, 200*time.Millisecond)
	if err != nil { //seguidor inalcanzable
		return false
	} else {
		//si se ha aceptado la operación
		if reply.Success {
			//registra la réplica
			nr.MatchIndex[nodo] = nr.NextIndex[nodo]
			nr.NextIndex[nodo]++
			nr.Mux.Lock()

			//Si suficientes nodos han confirmado la réplica de la entrada
			if nr.MatchIndex[nodo] > nr.CommitIndex {
				nr.numRespuestas++
				//si el núermo de respuestas alcanza una mayoría simple, 
				//se hace commit
				if nr.numRespuestas == len(nr.Nodos)/2 {
					nr.CommitIndex++ //incremento el índice de la entrada comprometida conocida
					nr.numRespuestas = 0
				}
			}

			nr.Mux.Unlock()
		}
		return true
	}
}

// Ejecutada por el líeder para enviar mensajes AppendEntries
//(tanto heartbeats como entradas de log) a todos los seguidores
// con la finalidad de replicar logs y corregir inconsistencias de logs
func sendAppendEntries(nr *NodoRaft) {
	var respuesta Results
	//a todos los nodos del sistema excepto al lider
	for i := 0; i < len(nr.Nodos); i++ {
		if i != nr.Yo {
			//Seguidor atrasado: si el índice del log del líder es mayor 
			//o igual l NextIndex esperado por el seguidor, aún no se 
			//ha replicado esta entrada en el nodo i
			if len(nr.Log)-1 >= nr.NextIndex[i] {
				//entrada a enviar al seguidor
				entry := LogEntry{nr.NextIndex[i], nr.Log[nr.NextIndex[i]].Term, nr.Log[nr.NextIndex[i]].Operation}
				//si no es la primera entrada del log, se envían también
				//los de la previa
				if nr.NextIndex[i] != 0 {
					prevIndex := nr.NextIndex[i] - 1
					prevTerm := nr.Log[prevIndex].Term
					//envia la operación
					go nr.enviarOperacion(i, &ArgAppendEntries{nr.CurrentTerm, nr.Yo, prevIndex, prevTerm, entry, nr.CommitIndex}, &respuesta)
				} else { //si es la primera entrada del log
					go nr.enviarOperacion(i, &ArgAppendEntries{nr.CurrentTerm, nr.Yo, -1, 0, entry, nr.CommitIndex}, &respuesta)
				}
			} else { //por el contrario, está actualizado, envía latido
				//mismo procedimiento
				if nr.NextIndex[i] != 0 { 
					prevIndex := nr.NextIndex[i] - 1
					prevTerm := nr.Log[prevIndex].Term
					go nr.enviarOperacion(i, &ArgAppendEntries{nr.CurrentTerm, nr.Yo, prevIndex, prevTerm, LogEntry{}, nr.CommitIndex}, &respuesta)
				} else {
					go nr.enviarOperacion(i, &ArgAppendEntries{nr.CurrentTerm, nr.Yo, -1, 0, LogEntry{}, nr.CommitIndex}, &respuesta)
				}
			}

		}
	}
}

// Devuelve "yo", mandato en curso y si este nodo cree ser lider
//
// Primer valor devuelto es el indice de este  nodo Raft el el conjunto de nodos 
// la operacion si consigue comprometerse.
// El segundo valor es el mandato en curso
// El tercer valor es true si el nodo cree ser el lider
// Cuarto valor es el lider, es el indice del líder si no es él
func (nr *NodoRaft) obtenerEstado() (int, int, bool, int) {
	var yo int = nr.Yo
	var mandato int = nr.CurrentTerm
	var esLider bool = (nr.IdLider == nr.Yo)
	var idLider int = nr.IdLider
	

	return yo, mandato, esLider, idLider
}

//resumen del estado del log comprometido
func (nr *NodoRaft) obtenerEstadoRegistro() (int, int) {
	indice := -1
	mandato := 0

	//indice y termino de la última entrada comprometida
	if len(nr.Log) != 0 {
		indice = nr.CommitIndex
		mandato = nr.Log[indice].Term
	}

	return indice, mandato
}

// El servicio que utilice Raft (base de datos clave/valor, por ejemplo)
// Quiere buscar un acuerdo de posicion en registro para siguiente operacion
// solicitada por cliente.

// Si el nodo no es el lider, devolver falso
// Sino, comenzar la operacion de consenso sobre la operacion y devolver en
// cuanto se consiga
// 
// No hay garantia que esta operacion consiga comprometerse en una entrada de
// de registro, dado que el lider puede fallar y la entrada ser reemplazada
// en el futuro.
// Primer valor devuelto es el indice del registro donde se va a colocar 
// la operacion si consigue comprometerse.
// El segundo valor es el mandato en curso
// El tercer valor es true si el nodo cree ser el lider
// Cuarto valor es el lider, es el indice del líder si no es él
func (nr *NodoRaft) someterOperacion(operacion TipoOperacion) (int, int,
															bool, int, string) {
	nr.Mux.Lock()
	//valores por defecto
	indice := -1	//indice del log donde se añade la operacion
	mandato := -1	//término actual
	EsLider := (nr.IdLider == nr.Yo)	//comprobación si nodo actual es lider
	idLider := -1
	valorADevolver := ""

	if !EsLider {
		//no es el lider
		nr.Mux.Unlock()
		idLider = nr.IdLider
	} else {
		//asigna indice y termido actual a la nueva entrada
		indice = len(nr.Log) //
		mandato = nr.CurrentTerm
		//crea una nueva entrada con la operacion solicitada
		newEntry := LogEntry{
			Index:     indice,
			Term:      mandato,
			Operation: operacion,
		}
		//añade la operacion al log local del lider
		nr.Log = append(nr.Log, newEntry)
		idLider = nr.IdLider
		nr.Mux.Unlock()
		nr.Logger.Printf("Lider %d somete operacion %v en indice %d\n",
						nr.Yo, operacion, indice)

		valorADevolver = <-nr.ResultOperacion //espero resultado de la máquina de estados

		nr.Logger.Println("Resultado de la operacion: ", valorADevolver)
	}
	return indice, mandato, EsLider, idLider, valorADevolver
}

// -----------------------------------------------------------------------
// LLAMADAS RPC al API
//
// Si no tenemos argumentos o respuesta estructura vacia (tamaño cero)
type Vacio struct{}

//Detiene el funcionamieto de un nodo
func (nr *NodoRaft) ParaNodo(args Vacio, reply *Vacio) error {
	defer nr.para()
	return nil
}

//Informacion del estado, usado para conocer el estado actual de un nodo
type EstadoParcial struct {
	Mandato	int //temino actual
	EsLider bool //true si el nodo cree ser el lider
	IdLider	int //identificador del lider conocido
}

//Estado parcial del nodo junto con el identificador que responde
type EstadoRemoto struct {
	IdNodo	int
	EstadoParcial
}

func (nr *NodoRaft) ObtenerEstadoNodo(args Vacio, reply *EstadoRemoto) error {
	nr.Mux.Lock()
	reply.IdNodo,reply.Mandato,reply.EsLider,reply.IdLider = nr.obtenerEstado()
	nr.Mux.Unlock()
	return nil
}

//resultado d euna operacion sometida
type ResultadoRemoto struct {
	ValorADevolver string //mensaje del resultado 
	IndiceRegistro int //indice del log donde se propuso la operacion
	EstadoParcial //estado del nodo emisor
}

//RPC invocado por el cliente
func (nr *NodoRaft) SometerOperacionRaft(operacion TipoOperacion,
												reply *ResultadoRemoto) error {
	reply.IndiceRegistro,reply.Mandato, reply.EsLider,
			reply.IdLider,reply.ValorADevolver = nr.someterOperacion(operacion)
	return nil
}

// -----------------------------------------------------------------------
// LLAMADAS RPC protocolo RAFT
//
// Structura de ejemplo de argumentos de RPC PedirVoto.
//
// Recordar
// -----------
// Nombres de campos deben comenzar con letra mayuscula !
//
// representación de los argumentos qie un nodo candidato envia a los demas
// servidores cuando solicita sus votos durante el proceso de eleccion de lider (RPC RequestVote)
type ArgsPeticionVoto struct {
	Term int			// termino actual del candidato que solicita el voto
	CandidateId int 	// id del candidato que solicita el voto
	LastLogIndex int	// indice de la ultima entrada en el log del candidato (para comparar quien está más actualizado)
	LastLogTerm int		// termino de la última entrada del log del candidato (para comprobar quien tiene el log mas reciente)
}

// Structura de ejemplo de respuesta de RPC PedirVoto,
//
// Recordar
// -----------
// Nombres de campos deben comenzar con letra mayuscula !
//
// respuesta que un servidor denvia al recibir una peticion de voto de un candidato
type RespuestaPeticionVoto struct {
	Term int 			// termino actual del seguidor (para que el candidato pueda actualizarse si está atrasado)
	VoteGranted bool	// true si el voto fue concedido al candidato
}

//Método que inicia el proceso de elección de lider
func (nr *NodoRaft) iniciarProcesoVotacion() {
	fmt.Println("Iniciando proceso de votacion con termino")
	var respuesta RespuestaPeticionVoto
	var lastLogIndex, lastLogTerm int

	if len(nr.Log) != 0 {
		//si no está vacío
		lastLogIndex = len(nr.Log) - 1
		lastLogTerm  = nr.Log[lastLogIndex].Term
	} else {
		lastLogIndex = -1
		lastLogTerm  = 0
	}
	for i := 0; i < len(nr.Nodos); i++ {
		if i != nr.Yo {
			go nr.enviarPeticionVoto(i, &ArgsPeticionVoto{nr.CurrentTerm, nr.Yo, lastLogIndex, lastLogTerm}, &respuesta)
		}
	}
	fmt.Println("Fin del proceso de votación")
}

// Metodo para RPC PedirVoto
func (nr *NodoRaft) PedirVoto(peticion *ArgsPeticionVoto,
										reply *RespuestaPeticionVoto) error {
	nr.Mux.Lock()
	defer nr.Mux.Unlock()
	fmt.Println("Peticion de voto recibida con termino: ", peticion.Term)

	//si el termino es menor o ya se ha votado en el termino se rechaza
	if (peticion.Term < nr.CurrentTerm) || (peticion.Term == nr.CurrentTerm && nr.VotedFor != peticion.CandidateId) {
		reply.Term = nr.CurrentTerm
		reply.VoteGranted = false
		fmt.Println("Voto rechazado")

	} else if peticion.Term > nr.CurrentTerm { //termino mayor actualiza el termino y concede el voto
		nr.CurrentTerm = peticion.Term //actualiza su termino
		//Comprobar el mejor líder con el último log
		//Solo votar por un candidato cuyo log esté al menos tan actualizado como el mío.
		//si no tiene log, o el candidato tiene mayor termino registrado o tiene el mismo termino pero mayor indice de log, concede voto
		if len(nr.Log) == 0 || (peticion.LastLogTerm > nr.Log[len(nr.Log)-1].Term) ||
			(peticion.LastLogTerm == nr.Log[len(nr.Log)-1].Term && peticion.LastLogIndex >= len(nr.Log)-1) {

			nr.VotedFor = peticion.CandidateId
			reply.Term = nr.CurrentTerm
			reply.VoteGranted = true
		} else { // no concede voto
			reply.Term = nr.CurrentTerm
			reply.VoteGranted = false
		}
		//si era lider o candidato, vuelve a ser seguidor
		if nr.State == Lider || nr.State == Candidato {
			fmt.Println("Era lider o candidato y me voy a convertir en follower")
			nr.serSeguidor <- true
			fmt.Println("Me convierto en follower")
		}
	}
	return nil	
}

// Contiene los argumentos que el líder envía al seguidor al invocar la RPC
// AppendEntries (para replicar logs o enviar heartbeats)
type ArgAppendEntries struct {
	Term int			// termino actual del lider (para actualización de followers)
	LeaderId int		// id del lider, para que los followers puedan redirigir clientes
	PrevLogIndex int	// indice de la entrada de log anterior a las nuevas (para comprobar coherencia)
	PrevLogTerm int		// termino de la entrada PrevLogIndex
	Entries	LogEntry	// entradas de log a añadir (vacío si es un heartbeat)
	LeaderCommit int	// indice de commit del lider
}

type Results struct {
	Term int 			// termino actual del seguidor (para que el lider pueda actualizarse si se ha atrasado)
	Success bool 		// true si el seguidor contiene una entrada que coincide con PrevLogIndex y PrevLogTerm
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

//Comprueba si el log es consistente con el que envía el líder
func logConsistente(nr *NodoRaft, prevIndex int, prevTerm int) bool {
	//si el índice previo está fuera del tamaño del log o el término
	//en este índice no coincide con el que se espera, devuelve falso
	if prevIndex > len(nr.Log)-1 || nr.Log[prevIndex].Term != prevTerm {
		return false
	} else {
		return true
	}
}

// Metodo de tratamiento de llamadas RPC AppendEntries
// Parámetros:
// 		- args: estructura de los campos del mensaje enviado por el lider
//		- result: estructura de respuesta que indica si el seguidor acepta la operacion
func (nr *NodoRaft) AppendEntries(args *ArgAppendEntries,
													  results *Results) error {
    nr.Mux.Lock()
	defer nr.Mux.Unlock()

	//Si el termino del lider es más antiguo que el del seguidor
	if args.Term < nr.CurrentTerm {
		//peticion de un lider obsoleto
		results.Term = nr.CurrentTerm
		results.Success = false //Indica fallo
		return nil
	} 
	
	//Término del líder mayor, actualizar estado local y volverse seguidor
	if args.Term > nr.CurrentTerm {
		//termino igual o mayor que el del seguidor
		nr.CurrentTerm = args.Term	//actualiza su termino al del lider
		nr.IdLider = args.LeaderId	//guarda la identidad del lider actual
		results.Term = nr.CurrentTerm	//informa del termino actualizado
		if nr.State == Lider { //si era líder, se vuelve seguidor
			nr.serSeguidor <- true
		} else { //Si el commit index del lider es mayor, se adopta
			if args.LeaderCommit > nr.CommitIndex {
				nr.CommitIndex = min(args.LeaderCommit, len(nr.Log)-1)
			}
			//latido válido
			nr.receivedHeartBeat <- true
			results.Success = true
		}

		return nil
	}

	nr.IdLider = args.LeaderId
	results.Term = nr.CurrentTerm

	//log vacío
	if len(nr.Log) == 0 {

		// Si llega una entrada, la añadimos
		if args.Entries != (LogEntry{}) {
			nr.Log = append(nr.Log, args.Entries)
		}

		results.Success = true
	} else if logConsistente(nr, args.PrevLogIndex, args.PrevLogTerm) {
		//si el log es consistente, aplica la entrada y elimina entradas
		//conflictivas antes de añadir la nueva
		if args.Entries != (LogEntry{}) {
			nr.Log = nr.Log[:args.PrevLogIndex+1] // eliminar versiones no consistentes
			nr.Log = append(nr.Log, args.Entries) //nueva
		}

		results.Success = true
	} else {
		//log no consistente
		results.Success = false
	}

	//actualiza índice comprometido local
	if args.LeaderCommit > nr.CommitIndex {
		nr.CommitIndex = min(args.LeaderCommit, len(nr.Log)-1)
	}
	nr.receivedHeartBeat <- true

	return nil
}


// ----- Metodos/Funciones a utilizar como clientes
//
//

// Ejemplo de código enviarPeticionVoto
//
// nodo int -- indice del servidor destino en nr.nodos[]
//
// args *RequestVoteArgs -- argumentos par la llamada RPC
//
// reply *RequestVoteReply -- respuesta RPC
//
// Los tipos de argumentos y respuesta pasados a CallTimeout deben ser
// los mismos que los argumentos declarados en el metodo de tratamiento
// de la llamada (incluido si son punteros
//
// Si en la llamada RPC, la respuesta llega en un intervalo de tiempo,
// la funcion devuelve true, sino devuelve false
//
// la llamada RPC deberia tener un timout adecuado.
//
// Un resultado falso podria ser causado por una replica caida,
// un servidor vivo que no es alcanzable (por problemas de red ?),
// una petición perdida, o una respuesta perdida
//
// Para problemas con funcionamiento de RPC, comprobar que la primera letra
// del nombre  todo los campos de la estructura (y sus subestructuras)
// pasadas como parametros en las llamadas RPC es una mayuscula,
// Y que la estructura de recuperacion de resultado sea un puntero a estructura
// y no la estructura misma.
//
func (nr *NodoRaft) enviarPeticionVoto(nodo int, args *ArgsPeticionVoto,
											reply *RespuestaPeticionVoto) bool {
	
	fmt.Printf("NodoRaft %d enviarPeticionVoto a nodo %d\n", nr.Yo, nodo)
	err := nr.Nodos[nodo].CallTimeout("NodoRaft.PedirVoto",
		args, &reply, 20 * time.Millisecond)
	if err != nil {
		nr.Logger.Printf("NodoRaft %d PeticionVoto fallo en nodo %d: %v\n",
			nr.Yo, nodo, err)
		return false
	} else {
		nr.Logger.Printf("NodoRaft %d PeticionVoto exito en nodo %d: %v\n",
			nr.Yo, nodo, reply)

		if reply.Term > nr.CurrentTerm {
			nr.CurrentTerm = reply.Term
			nr.serSeguidor <- true
		} else if reply.VoteGranted {
			nr.numVotos++
			if nr.numVotos >= len(nr.Nodos)/2+1 {
				nr.serLider <- true
			}
		}
	}
	return true
}

func vaciarCanal(ch chan bool) {
	for {
		select {
		case <-ch: // Consume un valor del canal
		default: // Sale si el canal está vacío
			return
		}
	}
}
