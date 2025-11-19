// Autor: Pedro Chaves y Beatriz Fetita
// Fecha: 2025-11-18
// Descripción: Implementación del protocolo Raft con comentarios completos,
// incluyendo especificaciones de precondiciones y postcondiciones para cada función.

// *** NOTA ***
// Documento generado automáticamente. Inserta tu nombre y adapta cualquier comentario según necesidad.

package raft
//

// API

// ===
// Este es el API que vuestra implementación debe exportar
// nodoRaft = NuevoNodo(...)
//   Crear un nuevo servidor del grupo de elección
// nodoRaft.Para()
//   Solicitar la parado de un servidor
// nodo.ObtenerEstado() (yo, mandato, esLider)
//   Solicitar a un nodo de elección por "yo", su mandato en curso,
//   y si piensa que es el msmo el lider
// nodoRaft.SometerOperacion(operacion interface()) (indice, mandato, esLider)

// type AplicaOperacion
import (
    "fmt"
    "io/ioutil"
    "log"
    "math/rand"
    "os"
    "sync"
    "time"
    "raft/internal/comun/rpctimeout"
)

// =====================================================================================
// Cabecera
// =====================================================================================
// Autor: [Tu Nombre]
// Fecha: 2025-11-18
// Fichero: raft.go
// Descripción: Implementación reorganizada y comentada de Raft. El fichero está
// agrupado por responsabilidades: definición de tipos, utilidades, estado del nodo,
// API pública, ciclo de vida, RPC del protocolo Raft, envío de mensajes y utilidades.
// NOTA: Actualiza el campo "Autor" con tu nombre real.

// =====================================================================================
// Constantes y tipos básicos
// =====================================================================================
const IntNOINICIALIZADO = -1	// Constante para fijar valor entero no inicializado
const kEnableDebugLogs = false 	//  false deshabilita por completo los logs de depuracion
const kLogToStdout = false 	// Poner a true para logear a stdout en lugar de a fichero
const kLogOutputDir = "./logs_raft/" 	// Cambiar esto para salida de logs en un directorio diferente

// State: rol del nodo
type State int64

const (
    Follower State = iota
    Candidate
    Leader
)

// TipoOperacion: representación de operaciones cliente (leer/escribir)
type TipoOperacion struct {
    Operacion string  // La operaciones posibles son "leer" y "escribir"
    Clave     string
    Valor     string  // en el caso de la lectura Valor = ""
}


// A medida que el nodo Raft conoce las operaciones de las  entradas de registro
// comprometidas, envía un AplicaOperacion, con cada una de ellas, al canal
// "canalAplicar" (funcion NuevoNodo) de la maquina de estados 
// AplicaOperacion: mensaje enviado a la máquina de estados cuando una entrada se commit
type AplicaOperacion struct {
    Indice    int	 // en la entrada de registro
    Operacion TipoOperacion
}


// cada nodo de Raft mantiene un array de LogEntry que contiene las operaciones
// pendientes o comprometidas que deben aplicarse a la maquina de estados replicada
// LogEntry: entrada en el registro de Raft
type LogEntry struct {
    Index     int	// indice de esta entrada dentro del log (iniciada en 1
    Term      int	// termino en el que el lider recibio esta entrada (para detectar inconsistencias en el log)
    Operation TipoOperacion	// comando que se debe aplicar a la maquina de estados
}

// =====================================================================================
// Tipos para RPC públicos y de protocolo
// =====================================================================================
// RPCs externos / API
// Si no tenemos argumentos o respuesta estructura vacia (tamaño cero)
type Vacio struct{}

//Informacion del estado, usado para conocer el estado actual de un nodo
type EstadoParcial struct {
    Mandato int //temino actual
    EsLider bool //si cree ser lider
    IdLider int //id del lider actual conocido
}
//Estado parcial del nodo junto con el identificador que responde
type EstadoRemoto struct {
    IdNodo int
    EstadoParcial
}
// Resultado de someter una operacion al nodo Raft
type ResultadoRemoto struct {
    ValorADevolver string   // resultado de la operacion
    IndiceRegistro int      // indice en el log donde se ha colocado la operacion
    EstadoParcial           // estado del nodo al responder
}

type EstadoRegistro struct {
    Index int
    Term  int
}

// representación de los argumentos qie un nodo candidato envia a los demas
// servidores cuando solicita sus votos durante el proceso de eleccion de lider (RPC RequestVote)
// RPC RequestVote
type ArgsPeticionVoto struct {
    Term         int    // termino actual del candidato que solicita el voto
    CandidateID  int    // id del candidato que solicita el voto
    LastLogIndex int    // indice de la ultima entrada en el log del candidato (para comparar quien está más actualizado)
    LastLogTerm  int    // termino de la última entrada del log del candidato (para comprobar quien tiene el log mas reciente)
}

// respuesta que un servidor denvia al recibir una peticion de voto de un candidato
type RespuestaPeticionVoto struct {
    Term        int     // termino actual del seguidor (para que el candidato pueda actualizarse si está atrasado)
    VoteGranted bool    // true si el voto fue concedido al candidato
}


// Contiene los argumentos que el líder envía al seguidor al invocar la RPC
// AppendEntries (para replicar logs o enviar heartbeats)
// RPC AppendEntries
type ArgAppendEntries struct {
    Term          int   // termino actual del lider (para actualización de followers)
    LeaderID      int   // id del lider, para que los followers puedan redirigir clientes
    PrevLogIndex  int   // indice de la entrada de log anterior a las nuevas (para comprobar coherencia)
    PrevLogTerm   int   // termino de la entrada PrevLogIndex
    EntryToCommit LogEntry  // entradas de log a añadir (vacío si es un heartbeat)
    LeaderCommit  int       // indice de commit del lider
}

type Results struct {
    Term    int     // termino actual del seguidor (para que el lider pueda actualizarse si se ha atrasado)
    Success bool    // true si el seguidor contiene una entrada que coincide con PrevLogIndex y PrevLogTerm
}

// =====================================================================================
// Utilidades generales
// =====================================================================================

// tiempoRandom: evita colisiones en elecciones
func tiempoRandom() time.Duration {
    return time.Duration(rand.Intn(900)+100) * time.Millisecond
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}

// =====================================================================================
// Estructura principal: estado del nodo Raft
// =====================================================================================
// NodoRaft contiene todo el estado necesario para ejecutar Raft.
// PRE: construir mediante NuevoNodo.
// POST: goroutine CicloDeVida en ejecución.
type NodoRaft struct {
    Mux sync.Mutex	// Mutex para proteger acceso a estado compartido

    // Red y identidad
    Nodos []rpctimeout.HostPort		//Lista de todos los nodos raft
	Yo    int           			// indice de este nodos en campo array "nodos"
	IdLider int						// id del lider actual conocido

	// Utilización opcional de este logger para depuración
	// Cada nodo Raft tiene su propio registro de trazas (logs)
    // Logging
    Logger *log.Logger

    // Rol y canales de coordinación entre goroutines
    NodeState          State	//estado actual: lider, candidato, seguidor
    ReceivedHearthbeat chan bool	// canal para notificar la recepción de un latido (heartbeat)
    SerFollower        chan bool	// canal para notificar el cambio a estado de seguidor
    SerLider           chan bool	// canal para notificar el cambio a estado de líder

    // Estado persistente/volátil
    CurrentTerm int // ultimo termino que ha visto el nodo (inicializado a 0, aumenta monotónicamente)
    VotedFor    int // id del candidato al que este nodo voto en el trmino actual (-1 si no ha votado)

    Log         []LogEntry	// log de entradas: cada entrada contiene un comando y el termino en el que fue recibido por el lider

    // Estado volatil en todos los servidores
	// (no se guarda en disco, se reinicia al arrancar)
	CommitIndex int				// indice de la entrada más alta conocida comprometida
	LastApplied int				// indice de la entrada más alta aplicada a la máquina de estados
    NextIndex []int				// para cada servidor: índice de la siguiente entrada de log que debe enviarse al ese seguidor
	MatchIndex []int			// para cada servidor: índice de la entrada más alta replicada en ese seguidor



    // Canales para aplicar operaciones y devolver resultados a cliente
	AplicaOperacion chan AplicaOperacion //Aplicacion de operaciones a la máquina de estados
	ResultOperacion chan string          //Canal para devolver el resultado de una operación

    // Contadores auxiliares
    NumVotos      int
    NumRespuestas int
}

// =====================================================================================
// CREACIÓN E INICIALIZACIÓN
// =====================================================================================

// Creacion de un nuevo nodo de eleccion
// Tabla de <Direccion IP:puerto> de cada nodo incluido a si mismo.
// <Direccion IP:puerto> de este nodo esta en nodos[yo]
// Todos los arrays nodos[] de los nodos tienen el mismo orden
// canalAplicar es un canal donde, en la practica 5, se recogerán las
// operaciones a aplicar a la máquina de estados. Se puede asumir que
// este canal se consumira de forma continúa.
// NuevoNodo() debe devolver resultado rápido, por lo que se deberían
// poner en marcha Gorutinas para trabajos de larga duracion
// NuevoNodo: inicializa campos y lanza CicloDeVida
func NuevoNodo(nodos []rpctimeout.HostPort, yo int, canalAplicarOperacion chan AplicaOperacion) *NodoRaft {
    nr := &NodoRaft{}

    nr.Nodos = nodos		//lista de todos los nodos
	nr.Yo = yo				//indice en la lista de nodos
	nr.IdLider = -1			//no hay lider conocido

    nr.NodeState = Follower	// estado inicial de seguidor
    nr.SerFollower = make(chan bool)
    nr.SerLider = make(chan bool)
    nr.ReceivedHearthbeat = make(chan bool)

    nr.CurrentTerm = 0	//termino inicial
    nr.VotedFor = -1	//no se ha votado en el termino actual

    nr.CommitIndex = -1 // Inicialmente no hay entradas comprometidas
	nr.LastApplied = -1 // Inicialmente no hay entradas aplicadas

    nr.NextIndex = make([]int, len(nodos))  // Inicialmente no hay entradas para enviar
	nr.MatchIndex = make([]int, len(nodos)) // Inicialmente no hay entradas replicadas

    nr.NumVotos = 0
    nr.NumRespuestas = 0

    nr.AplicarOperacion = canalAplicarOperacion
    nr.ResultadoOperacion = make(chan string)

    if kEnableDebugLogs {
        nombreNodo := nodos[yo].Host() + "_" + nodos[yo].Port()
        if kLogToStdout {
            nr.Logger = log.New(os.Stdout, nombreNodo+" -->> ", log.Lmicroseconds|log.Lshortfile)
        } else {
            os.MkdirAll(kLogOutputDir, os.ModePerm)
            logOutputFile, _ := os.OpenFile(fmt.Sprintf("%s/%s.txt", kLogOutputDir, nombreNodo), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
            nr.Logger = log.New(logOutputFile, nombreNodo+" -> ", log.Lmicroseconds|log.Lshortfile)
        }
    } else {
        nr.Logger = log.New(ioutil.Discard, "", 0)
    }
	// Añadir codigo de inicialización
    go CicloDeVida(nr)
    return nr
}

// =====================================================================================
// API PÚBLICA EXPUESTA (RPCs para clientes de la práctica)
// =====================================================================================

// ObtenerEstadoNodo: RPC que devuelve id, mandato, si cree ser lider y id del lider
func (nr *NodoRaft) ObtenerEstadoNodo(args Vacio, reply *EstadoRemoto) error {
    nr.Mux.Lock()
    reply.IdNodo, reply.Mandato, reply.EsLider, reply.IdLider = nr.obtenerEstado()
    nr.Mux.Unlock()
    return nil
}

// SometerOperacionRaft: RPC que permite a clientes someter operaciones
func (nr *NodoRaft) SometerOperacionRaft(operacion TipoOperacion, reply *ResultadoRemoto) error {
    reply.IndiceRegistro, reply.Mandato, reply.EsLider, reply.IdLider, reply.ValorADevolver = nr.someterOperacion(operacion)
    return nil
}

// ObtenerEstadoRegistro: RPC que devuelve índice y término del commit actual
func (nr *NodoRaft) ObtenerEstadoRegistro(args Vacio, reply *EstadoRegistro) error {
    nr.Mux.Lock()
    reply.Index, reply.Term = nr.obtenerEstadoRegistro()
    nr.Mux.Unlock()
    return nil
}

// ParaNodo: RPC para parar el proceso (utilizado por tests)
func (nr *NodoRaft) ParaNodo(args Vacio, reply *Vacio) error {
    defer nr.para()
    return nil
}

// =====================================================================================
// FUNCIONES AUXILIARES DE ESTADO / CONSULTA
// =====================================================================================

// Devuelve "yo", mandato en curso y si este nodo cree ser lider
// Primer valor devuelto es el indice de este  nodo Raft el el conjunto de nodos 
// la operacion si consigue comprometerse.
// El segundo valor es el mandato en curso
// El tercer valor es true si el nodo cree ser el lider
// Cuarto valor es el lider, es el indice del líder si no es él
// obtenerEstado: devuelve (yo, CurrentTerm, esLider, IdLider)
func (nr *NodoRaft) obtenerEstado() (int, int, bool, int) {
    yo := nr.Yo
    mandato := nr.CurrentTerm
    idLider := nr.IdLider
    esLider := (idLider == yo)
    return yo, mandato, esLider, idLider
}

// obtenerEstadoRegistro: devuelve commitIndex y término asociado (si existe)
func (nr *NodoRaft) obtenerEstadoRegistro() (int, int) {
    indice := -1
    mandato := 0
    //indice y termino de la última entrada comprometida
    if len(nr.Log) != 0 && nr.CommitIndex >= 0 {
        indice = nr.CommitIndex
        if indice < len(nr.Log) {
            mandato = nr.Log[indice].Term
        }
    }
    return indice, mandato
}

// para: detiene el proceso - en tests se usa exit
func (nr *NodoRaft) para() {
    go func() { time.Sleep(5 * time.Millisecond); os.Exit(0) }()
}

// =====================================================================================
// MÁQUINA DE ESTADOS: CicloDeVida
// =====================================================================================
// Esta función se ejecuta de manera concurrente para cada nodo.
// Implementa el bucle infinito que controla la máquina de estados de Raft:
// cada nodo puede estar en uno de tres estados: Seguidor, Candidato o Líder.
// CicloDeVida: goroutine principal que cambia entre Follower, Candidate y Leader.
// Comentarios críticos incluidos en las transiciones y acciones internas.
func CicloDeVida(nr *NodoRaft) {
    // Pequeña espera para sincronizar arranque en entornos de tests distribuidos
    time.Sleep(10000 * time.Millisecond)

    for {
        // Aplicación de entradas comprometidas (invariante: LastApplied <= CommitIndex)
        if nr.CommitIndex > nr.LastApplied {
			//si el número de entradas comprometidas es mayor a las aplicadas
            nr.LastApplied++//incrementa las aplicadas
			//crea la operación con índice la última en aplicar y la operación correspondiente
            op := AplicaOperacion{nr.LastApplied, nr.Log[nr.LastApplied].Operation}
            // CRÍTICO: AplicarOperacion es consumido por fuera; si esa goroutine falla, líder puede bloquear esperando resultado.
            nr.AplicarOperacion <- op
        }

        // --------------------
        // FOLLOWER
        // --------------------
        for nr.NodeState == Follower {
			fmt.Println("Nodo", nr.Yo, "es FOLLOWER en el mandato", nr.CurrentTerm)
            select {
			//Establecemos un timeout aleatorio
			//la base para iniciar una elección si no se recibe un heartbeat
            case <-nr.ReceivedHearthbeat:	//Si recibo un heartbeat es que hay un lider activo
                // Simplemente permanecemos follower al recibir latidos
				nr.NodeState = Follower
            case <-time.After(tiempoRandom()):
                // Timeout sin heartbeat -> iniciar elección
                nr.IdLider = -1
                nr.NodeState = Candidate	//cambio de estado a candidato
            }
        }

        // --------------------
        // CANDIDATE
        // --------------------
        for nr.NodeState == Candidate {
			fmt.Println("Nodo", nr.Yo, "es CANDIDATE en el mandato", nr.CurrentTerm)
            // Preparar candidatura
            nr.CurrentTerm++	//incrementa el termino actual antes de inciar una nueva votacion
            nr.VotedFor = nr.Yo	//se vota a si mismo
            nr.NumVotos = 1 // auto-voto

            // Inicia peticiones de voto en paralelo
            iniciarProcesoVotacion(nr)
			//timer para la duracion máxima de la votación
            timer := time.NewTimer(tiempoRandom())
            select {
            case <-nr.ReceivedHearthbeat:
				//se recive latido, hay un lider activo y se vuelve al estado de seguidor
                // Otro líder se ha manifestado
                nr.NodeState = Follower
            case <-nr.SerFollower:
				//recepcion de una señal para volver a ser seguidor
                // Reacción a detección de mandato superior
                nr.NodeState = Follower
            case <-nr.SerLider:
				//recepción de señal para ser lider
                // He ganado la elección: inicializar índices de replicación
                for i := range nr.Nodos {
                    if i != nr.Yo {
                        nr.NextIndex[i] = len(nr.Log)	//para saber desde donde se van a enviar las entradas
                        nr.MatchIndex[i] = -1		//inicializa los indices replicados
                    }
                }
                nr.NodeState = Leader
            case <-timer.C:
				//acaba el tiempo de eleccion, se inicia una nueva
                // Repetir elección
                nr.NodeState = Candidate
            }
        }

        // --------------------
        // LEADER
        // --------------------
        for nr.NodeState == Leader {
            nr.IdLider = nr.Yo

            // Envía append entries (operaciones pendientes o heartbeats)
            sendAppendEntries(nr)

            timer := time.NewTimer(50 * time.Millisecond)
            select {
            case <-nr.SerFollower:
                //señal para volver a ser seguidor (por la llegada de un termino mayor)
                // Al detectar mandato superior, retroceder a follower
                nr.NodeState = Follower
            case <-timer.C:
                // Al expirar el timer, aplicar entradas pendientes si commitIndex > LastApplied
                if nr.CommitIndex > nr.LastApplied {
                    //mantiene la máquina de estados actualizada
                    nr.LastApplied++
                    op := AplicaOperacion{nr.LastApplied, nr.Log[nr.LastApplied].Operation}
                    // CRÍTICO: se envía op a la máquina de estado y se espera por el resultado;
                    // si la máquina de estados aplica de forma síncrona y devuelve el valor a través
                    // del mismo canal, el líder bloquea hasta recibirlo.
                    nr.AplicarOperacion <- op
                    // Esperar la respuesta de la máquina de estados (puede bloquear)
                    res := <-nr.AplicarOperacion
                    // Devolver resultado al cliente que llamó SometerOperacion
                    nr.ResultadoOperacion <- res
                }
            }
        }
    }
}

// =====================================================================================
// LOGICA DE SOMETER OPERACIÓN (API LOCAL)
// =====================================================================================
// El servicio que utilice Raft (base de datos clave/valor, por ejemplo)
// Quiere buscar un acuerdo de posicion en registro para siguiente operacion
// solicitada por cliente.
// Si el nodo no es el lider, devolver falso
// Sino, comenzar la operacion de consenso sobre la operacion y devolver en
// cuanto se consiga
// No hay garantia que esta operacion consiga comprometerse en una entrada de
// de registro, dado que el lider puede fallar y la entrada ser reemplazada
// en el futuro.
// Primer valor devuelto es el indice del registro donde se va a colocar 
// la operacion si consigue comprometerse.
// El segundo valor es el mandato en curso
// El tercer valor es true si el nodo cree ser el lider
// Cuarto valor es el lider, es el indice del líder si no es él


// someterOperacion: añade la operación al log si es líder y espera su commit.
// PRE: operacion valida.
// POST: si líder, la entrada se inserta en log y la función espera hasta
// que la operación sea aplicada y retorna su resultado. Si no es líder, devuelve
// (indice=-1, mandato=-1, esLider=false, idLider=IdLiderActual, valorADevolver="").
func (nr *NodoRaft) someterOperacion(operacion TipoOperacion) (int, int, bool, int, string) {
    nr.Mux.Lock()
    // Evaluamos liderazgo al comienzo para evitar roces con cambio de estado
    esLider := nr.IdLider == nr.Yo
    if !esLider {
        idLider := nr.IdLider
        nr.Mux.Unlock()
        return -1, -1, false, idLider, ""
    }

	//añade la operacion al log local del lider
    indice := len(nr.Log)
    mandato := nr.CurrentTerm
    entry := LogEntry{indice, mandato, operacion}
    nr.Log = append(nr.Log, entry)
    nr.Mux.Unlock()     
    //Se gana tiempo solapando la operacion del wait del Rafto con el resto de operaciones
    // CRÍTICO: esperamos resultado a traves de canales. Si no se llega a commit,
    // esta espera puede bloquear indefinidamente. Debe implementarse un mecanismo
    // de timeout o cancelación si se desea mayor robustez.
    valor := <-nr.ResultadoOperacion    //espero resultado de la máquina de estados

    return indice, mandato, true, nr.Yo, valor
}

// =====================================================================================
// RPC DEL PROTOCOLO: PedirVoto (RequestVote)
// =====================================================================================

// PedirVoto: maneja solicitudes de voto desde candidatos.
// PRE: args correctamente formados.
// POST: reply.Term reflejará el mandato actualizado y VoteGranted indicará si concedió voto.
func (nr *NodoRaft) PedirVoto(peticion *ArgsPeticionVoto, reply *RespuestaPeticionVoto) error {
    nr.Mux.Lock()
    defer nr.Mux.Unlock()

    // Rechazar si el candidato está en un mandato más bajo
    if peticion.Term < nr.CurrentTerm {
        reply.Term = nr.CurrentTerm
        reply.VoteGranted = false
        return nil
    }

    // Si term > current, actualizar y convertirse en follower
    if peticion.Term > nr.CurrentTerm {
        nr.CurrentTerm = peticion.Term
        nr.VotedFor = -1
        if nr.NodeState != Follower {
            // Notificar cambio de rol -- operación asíncrona
            nr.SerFollower <- true
        }
    }
    //Comprobar el mejor líder con el último log
    //Solo votar por un candidato cuyo log esté al menos tan actualizado como el mío.
	//si no tiene log, o el candidato tiene mayor termino registrado o tiene el mismo termino pero mayor indice de log, concede voto
    // Verificar recency del log del candidato (según regla de Raft)
    upToDate := false
    if len(nr.Log) == 0 ||
        peticion.LastLogTerm > nr.Log[len(nr.Log)-1].Term ||
        (peticion.LastLogTerm == nr.Log[len(nr.Log)-1].Term && peticion.LastLogIndex >= len(nr.Log)-1) {
        upToDate = true
    }

    if (nr.VotedFor == -1 || nr.VotedFor == peticion.CandidateID) && upToDate {
        nr.VotedFor = peticion.CandidateID
        reply.VoteGranted = true
    } else {    // ya ha votado o log no está actualizado
        reply.VoteGranted = false
    }

    reply.Term = nr.CurrentTerm
    return nil
}

// =====================================================================================
// RPC DEL PROTOCOLO: AppendEntries
// =====================================================================================

// AppendEntries: maneja replicación y heartbeats.
// Comentarios críticos: condiciones de carrera, consistencia del log, y actualizacion de commit.
func (nr *NodoRaft) AppendEntries(args *ArgAppendEntries, results *Results) error {
    nr.Mux.Lock()
    defer nr.Mux.Unlock()

    // Caso: líder con term inferior que seguidor
    if args.Term < nr.CurrentTerm {
        results.Term = nr.CurrentTerm
        results.Success = false
        return nil
    }

    // Si el remitente tiene un término mayor, actualizar estado local y volverse seguidor
    if args.Term > nr.CurrentTerm {
        nr.CurrentTerm = args.Term
        nr.IdLider = args.LeaderID
        if nr.NodeState != Follower {
            nr.SerFollower <- true
        }
    }

    // Identificamos líder
    nr.IdLider = args.LeaderID
    results.Term = nr.CurrentTerm

    // Comprobación de consistencia: prevLog debe coincidir
    if args.PrevLogIndex != -1 {
        if args.PrevLogIndex > len(nr.Log)-1 || nr.Log[args.PrevLogIndex].Term != args.PrevLogTerm {
            results.Success = false
            return nil
        }
    }

    // Si hay una entrada nueva a replicar, ajustamos el log local
    if args.EntryToCommit != (LogEntry{}) {
        // Eliminar prefijo no consistente y añadir nueva entrada
        if args.PrevLogIndex+1 <= len(nr.Log)-1 {
            nr.Log = nr.Log[:args.PrevLogIndex+1]
        }
        nr.Log = append(nr.Log, args.EntryToCommit)
    }

    results.Success = true

    // Actualizar commitIndex local si el líder tiene un commit mayor
    if args.LeaderCommit > nr.CommitIndex {
        nr.CommitIndex = min(args.LeaderCommit, len(nr.Log)-1)
    }

    // Notificar recepción de heartbeat para reset de timeout
    select {
    case nr.ReceivedHearthbeat <- true:
    default:
        // Si nadie está esperando, omitimos para evitar bloqueo
    }

    return nil
}

// =====================================================================================
// ENVÍO DE MENSAJES: funciones que actúan como clientes RPC
// =====================================================================================

// enviarPeticionVoto: cliente RPC para PedirVoto
// CRÍTICO: manipula counters de votos sin lock; la función que llama debe
// garantizar sincronización o utilizar canales para comunicarse con la goroutine principal.
func (nr *NodoRaft) enviarPeticionVoto(nodo int, args *ArgsPeticionVoto, reply *RespuestaPeticionVoto) bool {
    err := nr.Nodos[nodo].CallTimeout("NodoRaft.PedirVoto", args, reply, 20*time.Millisecond)
    if err != nil {
        return false
    }

    // Si la réplica tiene un término mayor, forzamos transición a follower
    if reply.Term > nr.CurrentTerm {
        nr.Mux.Lock()
        nr.CurrentTerm = reply.Term
        nr.IdLider = -1
        nr.Mux.Unlock()
        // Notificar cambio
        nr.SerFollower <- true
        return true
    }

    if reply.VoteGranted {
        // NOTA CRÍTICA: actualizar NumVotos debe estar sincronizado con la goroutine que gestiona elecciones.
        nr.NumVotos++
        if nr.NumVotos > len(nr.Nodos)/2 {
            nr.SerLider <- true
        }
    }
    return true
}

// iniciarProcesoVotacion: lanza peticiones de voto paralelas
func iniciarProcesoVotacion(nr *NodoRaft) {
    var reply RespuestaPeticionVoto
    for i := range nr.Nodos {
        if i == nr.Yo {
            continue
        }
        if len(nr.Log) > 0 {
            // si el log no está vacío, enviar info del último entry
            lastIdx := len(nr.Log) - 1
            lastTerm := nr.Log[lastIdx].Term
            go nr.enviarPeticionVoto(i, &ArgsPeticionVoto{nr.CurrentTerm, nr.Yo, lastIdx, lastTerm}, &reply)
        } else {
            go nr.enviarPeticionVoto(i, &ArgsPeticionVoto{nr.CurrentTerm, nr.Yo, -1, 0}, &reply)
        }
    }
}

// enviarHearthbeat: envía AppendEntries sin entrada (heartbeat)
func (nr *NodoRaft) enviarHearthbeat(nodo int, args *ArgAppendEntries, reply *Results) bool {
    err := nr.Nodos[nodo].CallTimeout("NodoRaft.AppendEntries", args, reply, 20*time.Millisecond)
    if err != nil {
        return false
    }
    if reply.Term > nr.CurrentTerm {
        nr.Mux.Lock()
        nr.CurrentTerm = reply.Term
        nr.IdLider = -1
        nr.Mux.Unlock()
        nr.SerFollower <- true
    }
    return true
}

// enviarOperacion: envía AppendEntries con una entrada a un seguidor
// CRÍTICO: actualiza NextIndex/MatchIndex y puede avanzar CommitIndex; debe protegerse
// contra condiciones de carrera y asegurar que el líder calcula quorum correctamente.
func (nr *NodoRaft) enviarOperacion(nodo int, args *ArgAppendEntries, results *Results) bool {
    err := nr.Nodos[nodo].CallTimeout("NodoRaft.AppendEntries", args, results, 10*time.Millisecond)
    if err != nil { //seguidor inalcanzable
        return false
    }

    if results.Success {
        // Actualizar índices del seguidor
        nr.MatchIndex[nodo] = nr.NextIndex[nodo]
        nr.NextIndex[nodo]++

        // PROBLEMA CRÍTICO detectado en la versión original:
        // - usar NumRespuestas sin protección puede provocar commits incorrectos.
        // Aquí se protege con Mux al actualizar counters y commitIndex.
        nr.Mux.Lock()
        if nr.MatchIndex[nodo] > nr.CommitIndex { //Si suficientes nodos han confirmado la réplica de la entrada
            nr.NumRespuestas++
            //si el núermo de respuestas alcanza una mayoría simple, 
			//se hace commit
            if nr.NumRespuestas >= len(nr.Nodos)/2 {
                // AVANZAR commitIndex sólo si la entrada pertenece al mandato actual
                nr.CommitIndex++
                nr.NumRespuestas = 0
            }
        }
        nr.Mux.Unlock()
    } else {
        // Retroceder NextIndex para reintentar
        if nr.NextIndex[nodo] > 0 {
            nr.NextIndex[nodo]--
        }
    }
    return true
}



// Ejecutada por el líeder para enviar mensajes AppendEntries
//(tanto heartbeats como entradas de log) a todos los seguidores
// con la finalidad de replicar logs y corregir inconsistencias de logs
// sendAppendEntries: decide si enviar operaciones pendientes o heartbeats
func sendAppendEntries(nr *NodoRaft) {
    var reply Results
    for i := range nr.Nodos {
        if i == nr.Yo {
            continue
        }
        //Seguidor atrasado: si el índice del log del líder es mayor 
		//o igual l NextIndex esperado por el seguidor, aún no se 
		//ha replicado esta entrada en el nodo i
        // Si hay una operación que este seguidor aún no ha recibido
        if len(nr.Log) > nr.NextIndex[i] {
            //entrada a enviar al seguidor
            entry := nr.Log[nr.NextIndex[i]]

            prevIdx := nr.NextIndex[i] - 1
            prevTerm := 0
            //si no es la primera entrada del log, se envían también
			//los de la previa
            if prevIdx >= 0 {
                prevTerm = nr.Log[prevIdx].Term
            }
			//envia la operación
            go nr.enviarOperacion(i, &ArgAppendEntries{
                Term:          nr.CurrentTerm,
                LeaderID:      nr.Yo,
                PrevLogIndex:  prevIdx,
                PrevLogTerm:   prevTerm,
                EntryToCommit: entry,
                LeaderCommit:  nr.CommitIndex,
            }, &reply)

        } else { //por el contrario, está actualizado, envía latido
            // No hay entradas nuevas: enviar heartbeat
            //mismo procedimiento
            prevIdx := nr.NextIndex[i] - 1
            prevTerm := 0
            if prevIdx >= 0 {
                prevTerm = nr.Log[prevIdx].Term
            }
            go nr.enviarHearthbeat(i, &ArgAppendEntries{
                Term:          nr.CurrentTerm,
                LeaderID:      nr.Yo,
                PrevLogIndex:  prevIdx,
                PrevLogTerm:   prevTerm,
                EntryToCommit: LogEntry{},
                LeaderCommit:  nr.CommitIndex,
            }, &reply)
        }
    }
}
