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

	// Estado volátil en líderes (reiniciado tras cada elección)
	NextIndex []int				// para cada servidor: índice de la siguiente entrada de log que debe enviarse al ese seguidor
	MatchIndex []int			// para cada servidor: índice de la entrada más alta replicada en ese seguidor

	ApplyCh chan AplicaOperacion // canal para enviar operaciones comprometidas a la máquina de estados

	opComprometida chan string	// canal para notificar que una operación se ha comprometido
	receivedHeartbeat chan bool	// canal para notificar la recepción de un latido (heartbeat)

	termUpdated chan bool		// canal para notificar que el término ha sido actualizado
	serSeguidor chan bool		// canal para notificar el cambio a estado de seguidor
	serLider chan bool			// canal para notificar el cambio a estado de líder

	EnVotacion bool			// indica si el nodo está en proceso de votación
	
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

	nr.State = Follower		// estado inicial de seguidor

	nr.CurrentTerm = 0		//termino inicial
	nr.VotedFor = -1		//no se ha votado en el termino actual

	nr.CommitIndex = -1 // Inicialmente no hay entradas comprometidas
	nr.LastApplied = -1 // Inicialmente no hay entradas aplicadas

	nr.NextIndex = make([]int, len(nodos))  // Inicialmente no hay entradas para enviar
	nr.MatchIndex = make([]int, len(nodos)) // Inicialmente no hay entradas replicadas

	nr.opComprometida = make(chan string)
	nr.receivedHeartBeat = make(chan bool)

	nr.termUpdated = make(chan bool, len(nodos))
	nr.serFollower = make(chan bool)
	nr.serLider = make(chan bool)
	nr.EnVotacion = false		//actualmente en proceso de votación

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

// Funcion para obtener un timeout aleatorio de entre 100 y 500ms
func obtenerTimeoutAleatorio() time.Duration {
	return time.Duration(100+rand.Intn(400)) * time.Millisecond
}

func CicloDeVida(nr *NodoRaft) {
	time.Sleep(10000 * time.Millisecond)
	for {
		fmt.Printf("NodoRaft %d CicloDeVida\n", nr.Yo)
		switch nr.State {
		case Seguidor:
			fmt.Println("Soy follower")
			//Establecemos un timeout aleatorio entre 1 y 2 segundos
			//la base para iniciar una elección si no se recibe un heartbeat
			timeout := time.Duration(1+rand.Intn(2)) * time.Second
			select {
			case <-time.After(timeout):
				//expita el timeout
				fmt.Println("Timeout salta, paso a ser candidato")
				nr.State = Candidato	//cambio de estado a candidato
				nr.IdLider = -1 		//No hay lider concido
			case <-nr.receivedHeartbeat: //Si recibo un heartbeat es que hay un lider activo
				fmt.Println("Heartbeat recibido")
				nr.State = Seguidor		//se mantiene como seguidor
			}
		case Candidato:
			fmt.Println("Soy Candidato")
			nr.CurrentTerm++		//incrementa el termino actual antes de inciar una nueva votacion
			nr.VotedFor = nr.Yo		//se vota a si mismo

			go nr.iniciarProcesoVotacion()
			//timer para la duracion máxima de la votación
			timer := time.NewTimer(obtenerTimeoutAleatorio())
			select {
			case <-nr.receivedHeartBeat:
				//se recive latido, hay un lider activo y se vuelve al estado de seguidor
				nr.State = Seguidor

			case <-timer.C:
				//acaba el tiempo de eleccion, se inicia una nueva
				nr.State = Candidato

			case <-nr.serFollower:
				//recepcion de una señal para volver a ser seguidor
				nr.State = Seguidor

			case <-nr.serLider:
				//recepción de señal para ser lider
				nr.State = Lider
			}
		case Lider:
			fmt.Println("Soy Lider")
			nr.IdLider = nr.Yo
			sendHeartBeatTodos(nr)
			timer := time.NewTimer(100 * time.Millisecond)

			select {
			case <-nr.serFollower:
				nr.State = Seguidor
			case <-timer.C:
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

// Funcion para enviar un heartbeat a todos los nodos
func sendHeartBeatTodos(nr *NodoRaft) {
	var reply Results
	for i := 0; i < len(nr.Nodos); i++ {
		if i != nr.Yo {
			go nr.enviarHeartBeat(i, &ArgAppendEntries{nr.CurrentTerm, nr.Yo, 0, 0, LogEntry{}, nr.CommitIndex}, &reply)
		}
	}
}

// Funcion para enviar un heartbeat a un nodo
func (nr *NodoRaft) enviarHeartBeat(nodo int, args *ArgAppendEntries, reply *Results) bool {
	err := nr.Nodos[nodo].CallTimeout("NodoRaft.AppendEntries", args, reply, 200*time.Millisecond)
	if err != nil {
		return false
	} else {
		if reply.Term > nr.CurrentTerm { //Si el termino del que le he mandado el heartbeat es mayor que el mio
			nr.Mux.Lock()               // Bloquear acceso a estado compartido
			nr.CurrentTerm = reply.Term //Actualizo mi termino
			nr.serFollower <- true      //Me convierto en follower
			nr.IdLider = -1             //No hay lider, hay que elegir uno nuevo
			nr.Mux.Unlock()             // Desbloquear acceso a estado compartido
		}
		return true
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
	indice := -1
	mandato := -1
	EsLider := (nr.State == Lider)
	idLider := -1
	valorADevolver := ""

	// Vuestro codigo aqui
	if nr.State != Lider {
		idLider = nr.IdLider
		nr.Mux.Unlock()
	} else {
		indice = len(nr.Log)
		mandato = nr.CurrentTerm
		newEntry := LogEntry{
			Index:     indice,
			Term:      mandato,
			Operation: operacion,
		}
		nr.Log = append(nr.Log, newEntry)
		nr.Mux.Unlock()
		nr.Logger.Printf("Lider %d somete operacion %v en indice %d\n",
						nr.Yo, operacion, indice)
		commitCh := make(chan bool)
		commited := 1
		noCommited := 0
		for i := 0; i < len(nr.Nodos); i++ {
			if i != nr.Yo {
				go llamadaAppendEntriesSometer(nr, i, newEntry, commitCh)
			}
		}
		mayoriaSimple := (len(nr.Nodos) / 2) + 1
		for (commited < mayoriaSimple) && (noCommited < mayoriaSimple) {
			result := <-commitCh
			if result {
				commited++
			} else {
				noCommited++
			}
		}
		if commited >= mayoriaSimple {
			nr.CommitIndex ++ // Commitear la entrada
			nr.Logger.Printf("Lider %d operacion %v comprometida en indice %d\n",
						nr.Yo, operacion, indice)
		} else {
			nr.Logger.Printf("Lider %d operacion %v NO comprometida en indice %d\n",
						nr.Yo, operacion, indice)
		}
	}
	return indice, mandato, EsLider, idLider, valorADevolver
}

func llamadaAppendEntriesSometer(nr *NodoRaft, nodo int,
								entry LogEntry, commitCh chan bool) {
	var respuesta Results
	err := nr.Nodos[nodo].CallTimeout("NodoRaft.AppendEntries",
		&ArgAppendEntries{
			Term:         nr.CurrentTerm,
			LeaderId:     nr.Yo,
			PrevLogIndex: len(nr.Log) - 1,
			PrevLogTerm:  nr.Log[entry.Index - 1].Term,
			Entries:      entry,
			LeaderCommit: nr.CommitIndex,
		},
		&respuesta,
		200 * time.Millisecond)
	if err != nil {
		nr.Logger.Printf("Lider %d AppendEntries fallo en nodo %d para indice %d: %v\n",
			nr.Yo, nodo, entry.Index, err)
		commitCh <- false
	} else {
		if respuesta.Success {
			nr.Logger.Printf("Lider %d AppendEntries exito en nodo %d para indice %d\n",
				nr.Yo, nodo, entry.Index)
			commitCh <- true
		} else {
			nr.Logger.Printf("Lider %d AppendEntries RECHAZADO en nodo %d para indice %d\n",
				nr.Yo, nodo, entry.Index)
			commitCh <- false
		}
	}
}

// -----------------------------------------------------------------------
// LLAMADAS RPC al API
//
// Si no tenemos argumentos o respuesta estructura vacia (tamaño cero)
type Vacio struct{}

func (nr *NodoRaft) ParaNodo(args Vacio, reply *Vacio) error {
	defer nr.para()
	return nil
}

type EstadoParcial struct {
	Mandato	int
	EsLider bool
	IdLider	int
}

type EstadoRemoto struct {
	IdNodo	int
	EstadoParcial
}

func (nr *NodoRaft) ObtenerEstadoNodo(args Vacio, reply *EstadoRemoto) error {
	reply.IdNodo,reply.Mandato,reply.EsLider,reply.IdLider = nr.obtenerEstado()
	return nil
}

type ResultadoRemoto struct {
	ValorADevolver string
	IndiceRegistro int
	EstadoParcial
}

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

/*
 */
func (nr *NodoRaft) iniciarProcesoVotacion() {
	nr.EnVotacion = true
	args := ArgsPeticionVoto{
		Term: nr.CurrentTerm,
		CandidateId: nr.Yo,
		LastLogIndex: nr.CommitIndex,
		LastLogTerm: nr.LastTermApplied,
	}
	fmt.Println("Iniciando proceso de votacion con termino: ", args.Term, " y candidato: ", args.CandidateId, " y ultimo indice logeado: ", args.LastLogIndex, " y ultimo periodo logeado: ", args.LastLogTerm)

	voteResult := make(chan bool, len(nr.Nodos)-1)

	var respuesta RespuestaPeticionVoto

	for i := 0; i < len(nr.Nodos); i++ {
		if i != nr.Yo {
			go nr.pedirVotoPersistente(i, &args, &respuesta, voteResult)
		}
	}

	mayoriaSimple := (len(nr.Nodos) / 2) + 1
	votosFavorables := 1
	votosNegados := 0

	for (votosFavorables < mayoriaSimple) && (votosNegados < mayoriaSimple) {
		voto := <-voteResult
		fmt.Println("Recibido voto: ", voto)
		if voto {
			votosFavorables++
		} else {
			votosNegados++
		}

	}

	if votosFavorables >= mayoriaSimple {
		nr.serLider <- true
	} else {
		nr.serFollower <- true
	}
	nr.EnVotacion = false
}

func (nr *NodoRaft) pedirVotoPersistente(nodo int, args *ArgsPeticionVoto,
	reply *RespuestaPeticionVoto, votoResult chan bool) {
	votacionTerminada := false
	vaciarCanal(nr.termUpdated)
	for !votacionTerminada && nr.State != Leader { //Mientras no haya terminado la votacion y no sea lider
		select {
		//Se ha terminado el periodo de votacion
		case <-nr.termUpdated:
			fmt.Println("Se ha terminado el periodo de votacion")
			votacionTerminada = true
			nr.EnVotacion = false
			votoResult <- false
		default:
			votacionTerminada = nr.enviarPeticionVoto(nodo, args, reply)
			fmt.Println("Votacion terminada: ", votacionTerminada)
			if votacionTerminada {
				if reply.Granted {
					fmt.Println("Voto concedido por nodo: ", nodo)
					votoResult <- true
				} else {
					fmt.Println("Voto denegado por nodo: ", nodo)
					votoResult <- false
				}
				nr.EnVotacion = false
			} else {
				fmt.Println("Ha habido un error en la RPC de PedirVoto")
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// Metodo para RPC PedirVoto
//
func (nr *NodoRaft) PedirVoto(peticion *ArgsPeticionVoto,
										reply *RespuestaPeticionVoto) error {
	nr.Mux.Lock()
	defer nr.Mux.Unlock()
	fmt.Println("Peticion de voto recibida con termino: ", peticion.Term)

	
	if (peticion.Term < nr.CurrentTerm) || (peticion.Term == nr.CurrentTerm && nr.VotedFor != peticion.CandidateId) {
		reply.Term = nr.CurrentTerm
		reply.Granted = false
		fmt.Println("Voto rechazado")

	} else if peticion.Term > nr.CurrentTerm {
		nr.CurrentTerm = peticion.Term
		nr.VotedFor = peticion.CandidateId
		reply.Term = nr.CurrentTerm
		reply.Granted = true
		fmt.Println("Voto concedido")
		if nr.EnVotacion {
			fmt.Println("Estoy en votacion y mando al canal")
			nr.termUpdated <- true
		}
		if nr.State == Leader || nr.State == Candidate {
			fmt.Println("Era lider o candidato y me voy a convertir en follower")
			nr.serFollower <- true
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

// Metodo de tratamiento de llamadas RPC AppendEntries
func (nr *NodoRaft) AppendEntries(args *ArgAppendEntries,
													  results *Results) error {
    nr.Mux.Lock()
	defer nr.Mux.Unlock()

	if args.Term < nr.CurrentTerm {
		results.Term = nr.CurrentTerm
		results.Success = false
	} else {
		nr.CurrentTerm = args.Term
		nr.IdLider = args.LeaderId
		results.Term = nr.CurrentTerm
		if args.Entries != (LogEntry{}) {
			// Añadir entradas al log
			nr.Log = append(nr.Log, args.Entries)
		}
		if args.Term > nr.CurrentTerm {
			if nr.State == Lider {
				nr.serSeguidor <- true
			} else {
				if args.LeaderCommit > nr.CommitIndex {
					nr.CommitIndex = min(args.LeaderCommit, len(nr.Log)-1)
				}
				nr.receivedHeartbeat <- true
				results.Success = true
			}
		} else {
			nr.receivedHeartbeat <- true
			results.Success = true
		}
	}

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
		args, &reply, 200 * time.Millisecond)
	if err != nil {
		nr.Logger.Printf("NodoRaft %d PeticionVoto fallo en nodo %d: %v\n",
			nr.Yo, nodo, err)
		return false
	} else {
		nr.Logger.Printf("NodoRaft %d PeticionVoto exito en nodo %d: %v\n",
			nr.Yo, nodo, reply)
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