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

// CicloDeVida gestiona el comportamiento principal de un nodo Raft.
//
// Esta función se ejecuta de manera concurrente para cada nodo.
// Implementa el bucle infinito que controla la máquina de estados de Raft:
// cada nodo puede estar en uno de tres estados: Seguidor, Candidato o Líder.
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
			case <-nr.receivedHeartBeat: //Si recibo un heartbeat es que hay un lider activo
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

			case <-nr.serSeguidor:
				//recepcion de una señal para volver a ser seguidor
				nr.State = Seguidor

			case <-nr.serLider:
				//recepción de señal para ser lider
				nr.State = Lider
			}
		case Lider:
			fmt.Println("Soy Lider")
			nr.IdLider = nr.Yo		//lider actual
			//Envía heartbeats (AppendEntries vacio) a todos los seguidores para que inicien votación
			sendHeartBeat(nr)
			// timer para la frecuencia de envio de latido
			timer := time.NewTimer(100 * time.Millisecond)

			select {
			case <-nr.serSeguidor:
				//señal para volver a ser seguidor (por la llegada de un termino mayor)
				nr.State = Seguidor
			case <-timer.C:
				//expira el timer, sigue siendo líder y mandando latido
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
func sendHeartBeat(nr *NodoRaft) {
	for i := 0; i < len(nr.Nodos); i++ {
		//no se envia latido al mismo proceso lider
		if i != nr.Yo {
			var reply Results //repuesta local para evitar condiciones de carrera
			go nr.enviarHeartBeat(i, &ArgAppendEntries{nr.CurrentTerm, nr.Yo, 0, 0, LogEntry{}, nr.CommitIndex}, &reply)
			//Mas adelante se gestiona el reply
		}
	}
}

// Funcion para enviar un heartbeat a un nodo
// devuelve true si la llamada a AppendEntries ha sido correcta
func (nr *NodoRaft) enviarHeartBeat(nodo int, args *ArgAppendEntries, reply *Results) bool {
	err := nr.Nodos[nodo].CallTimeout("NodoRaft.AppendEntries", args, reply, 200*time.Millisecond)
	if err != nil { // si no se ha podido conectar con el nodo
		return false
	} else {
		if reply.Term > nr.CurrentTerm { //Si el termino del nodo remoto es mayor que el de ahora, está desactualizado y se convierte en seguidor
			nr.Mux.Lock()               // Bloquear acceso a estado compartido
			nr.CurrentTerm = reply.Term //Actualizo mi termino
			nr.serSeguidor <- true      //Me convierto en follower
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
	//valores por defecto
	indice := -1	//indice del log donde se añade la operacion
	mandato := -1	//término actual
	EsLider := (nr.IdLider == nr.Yo)	//comprobación si nodo actual es lider
	idLider := -1
	valorADevolver := ""

	// Vuestro codigo aqui
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
		commitCh := make(chan bool) //canal para recibir confirmaciones de los seguidores
		commited := 1 //iniciando contando la entrada del lider
		noCommited := 0 //respuestas fallidas
		//intenta replicar la nueva entrada en todos los seguidores
		for i := 0; i < len(nr.Nodos); i++ {
			if i != nr.Yo {
				go llamadaAppendEntriesSometer(nr, i, newEntry, commitCh)
			}
		}
		mayoriaSimple := (len(nr.Nodos) / 2) + 1
		//espera confirmación hasta alcanzar una mayoria simple de aciertos o fallos
		for (commited < mayoriaSimple) && (noCommited < mayoriaSimple) {
			result := <-commitCh //bloqueo hasta recibir respuesta
			if result {
				commited++
			} else {
				noCommited++
			}
		}
		//verifica que la operacion se haya comprometido en la mayoría
		if commited >= mayoriaSimple {
			nr.CommitIndex ++ // Commitear la entrada
			valorADevolver = "Operacion realizada con exito"
		} else {
			valorADevolver = "Operacion no realizada"
		}
	}
	return indice, mandato, EsLider, idLider, valorADevolver
}

//Esta función se encarga de enviar RPC AppendEntries a un nodo seguidor para
//replicar la nueva entrada del log, ejecutada de forma concurrente
func llamadaAppendEntriesSometer(nr *NodoRaft, nodo int,
								entry LogEntry, commitCh chan bool) {
	var respuesta Results
	err := nr.Nodos[nodo].CallTimeout("NodoRaft.AppendEntries",
		&ArgAppendEntries{
			Term:         nr.CurrentTerm, //termino actual del lider
			LeaderId:     nr.Yo,		  //id del lider emisor
			PrevLogIndex: len(nr.Log) - 1, //indice previo
			PrevLogTerm:  nr.LastApplied, //termino previo (ultima entrada registrada)
			Entries:      entry,	//entrada del log a replicar 
			LeaderCommit: nr.CommitIndex, //ultimo indice comprometido conocido
		},
		&respuesta,
		200 * time.Millisecond)
	//en función del resultado de la llamada anterior
	if err != nil {
		//fallo de comunicación
		nr.Logger.Printf("Lider %d AppendEntries fallo en nodo %d para indice %d: %v\n",
			nr.Yo, nodo, entry.Index, err)
		commitCh <- false
	} else {
		//Respuesta de forma correcta
		if respuesta.Success {
			//Acepta la entrada
			nr.Logger.Printf("Lider %d AppendEntries exito en nodo %d para indice %d\n",
				nr.Yo, nodo, entry.Index)
			commitCh <- true
		} else {
			//entrada rechazada (por termino o log inconsistente)
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
	reply.IdNodo,reply.Mandato,reply.EsLider,reply.IdLider = nr.obtenerEstado()
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
	nr.EnVotacion = true //nodo el votación 
	//argumentos de la peticion de voto a enviar a todos los nodos
	args := ArgsPeticionVoto{
		Term: nr.CurrentTerm, //termino actual
		CandidateId: nr.Yo, //identificador del candidato
		LastLogIndex: nr.CommitIndex, //indice del ultimo registro confirmado
		LastLogTerm: nr.LastTermApplied, //termino del ultimo registro confirmado
	}
	fmt.Println("Iniciando proceso de votacion con termino: ", args.Term, " y candidato: ", args.CandidateId, " y ultimo indice logeado: ", args.LastLogIndex, " y ultimo periodo logeado: ", args.LastLogTerm)

	//canal para recibir los resultados de los votos
	voteResult := make(chan bool, len(nr.Nodos)-1)

	var respuesta RespuestaPeticionVoto //estructura para la respuesta
	//envío de solicitud de voto
	for i := 0; i < len(nr.Nodos); i++ {
		if i != nr.Yo {
			go nr.pedirVotoPersistente(i, &args, &respuesta, voteResult)
		}
	}

	mayoriaSimple := (len(nr.Nodos) / 2) + 1
	votosFavorables := 1 //se vota a si mismo
	votosNegados := 0

	//espera a la mayoria simple 
	for (votosFavorables < mayoriaSimple) && (votosNegados < mayoriaSimple) {
		voto := <-voteResult //espera resultados de votacion
		fmt.Println("Recibido voto: ", voto)
		if voto {
			votosFavorables++
		} else {
			votosNegados++
		}

	}

	//si alcanza la mayoria se convierte en lider
	if votosFavorables >= mayoriaSimple {
		nr.serLider <- true
	} else { //si no, vuelve a ser seguidor
		nr.serSeguidor <- true
	}
	nr.EnVotacion = false //fin del proceso de votación
	fmt.Println("Fin del proceso de votación")
}

//Función para mandar votos a un nodo de forma repetida hasta que 
//se obtiene una respuesta o termina la votacion
func (nr *NodoRaft) pedirVotoPersistente(nodo int, args *ArgsPeticionVoto,
	reply *RespuestaPeticionVoto, votoResult chan bool) {
	votacionTerminada := false
	vaciarCanal(nr.termUpdated) //vacia el canal antes de empezar
	for !votacionTerminada && nr.State != Lider { //Mientras no haya terminado la votacion y no sea lider
		select {
		//Se ha terminado el periodo de votacion
		case <-nr.termUpdated:
			fmt.Println("Se ha terminado el periodo de votacion")
			votacionTerminada = true
			nr.EnVotacion = false
			votoResult <- false
		default:
			//intenta enviar peticion de voto
			votacionTerminada = nr.enviarPeticionVoto(nodo, args, reply)
			fmt.Println("Votacion terminada: ", votacionTerminada)
			if votacionTerminada {
				//se ha obtenido respuesta RPC
				if reply.VoteGranted {
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
		nr.CurrentTerm = peticion.Term
		nr.VotedFor = peticion.CandidateId
		reply.Term = nr.CurrentTerm
		reply.VoteGranted = true
		fmt.Println("Voto concedido")
		//si estaba en votación, ntofic afin del periodo de votacion
		if nr.EnVotacion {
			fmt.Println("Estoy en votacion y mando al canal")
			nr.termUpdated <- true
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
	} else {
		//termino igual o mayor que el del seguidor
		nr.CurrentTerm = args.Term	//actualiza su termino al del lider
		nr.IdLider = args.LeaderId	//guarda la identidad del lider actual
		results.Term = nr.CurrentTerm	//informa del termino actualizado
		if args.Entries != (LogEntry{}) {
			//caso en el que no está vacío
			// Añadir entradas al log
			nr.Log = append(nr.Log, args.Entries)
		}
		if args.Term > nr.CurrentTerm {
			if nr.State == Lider {
				//
				nr.serSeguidor <- true
			} else {
				if args.LeaderCommit > nr.CommitIndex {
					nr.CommitIndex = min(args.LeaderCommit, len(nr.Log)-1)
				}
				nr.receivedHeartBeat <- true
				results.Success = true
			}
		} else {
			//el termino del lider coincide con el del seguidor, se marca latido recibido
			nr.receivedHeartBeat <- true
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
		args, &reply, 20 * time.Millisecond)
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
