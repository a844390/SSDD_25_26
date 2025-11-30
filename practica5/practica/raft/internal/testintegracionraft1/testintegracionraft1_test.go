package testintegracionraft1

import (
	"fmt"
	"raft/internal/comun/check"

	//"log"
	"math/rand"
	//"os"
	//"path/filepath"
	"strconv"
	"testing"
	"time"

	"raft/internal/comun/rpctimeout"
	"raft/internal/despliegue"
	"raft/internal/raft"
)

const (

	//hosts
	MAQUINA1 = "192.168.3.8"
	MAQUINA2 = "192.168.3.8"
	MAQUINA3 = "192.168.3.8"

	//puertos
	PUERTOREPLICA1 = "31250"
	PUERTOREPLICA2 = "31251"
	PUERTOREPLICA3 = "31252"

	//nodos replicas
	REPLICA1 = MAQUINA1 + ":" + PUERTOREPLICA1
	REPLICA2 = MAQUINA2 + ":" + PUERTOREPLICA2
	REPLICA3 = MAQUINA3 + ":" + PUERTOREPLICA3

	// paquete main de ejecutables relativos a PATH previo
	EXECREPLICA = "cmd/srvraft/main.go"

	// comandos completo a ejecutar en máquinas remota con ssh. Ejemplo :
	// 				cd $HOME/raft; go run cmd/srvraft/main.go 127.0.0.1:29001

	// Ubicar, en esta constante, nombre de fichero de vuestra clave privada local
	// emparejada con la clave pública en authorized_keys de máquinas remotas

	PRIVKEYFILE = "id_ed25519"
)

// PATH de los ejecutables de modulo golang de servicio Raft
var PATH string = "/misc/alumnos/sd/sd2526/a840269/practica5/raft"

// go run cmd/srvraft/main.go 0 127.0.0.1:29001 127.0.0.1:29002 127.0.0.1:29003
//var EXECREPLICACMD string = "cd " + PATH + "; /usr/local/go/bin/go run " + EXECREPLICA
var EXECREPLICACMD string = "cd " + PATH + ";/usr/bin/go mod tidy;/usr/bin/go run " + PATH + "/" + EXECREPLICA

// TEST primer rango
func TestPrimerasPruebas(t *testing.T) { // (m *testing.M) {
	// <setup code>
	// Crear canal de resultados de ejecuciones ssh en maquinas remotas
	cfg := makeCfgDespliegue(t,
		3,
		[]string{REPLICA1, REPLICA2, REPLICA3},
		[]bool{true, true, true})

	// tear down code
	// eliminar procesos en máquinas remotas
	defer cfg.stop()

	// Run test sequence

	// Test1 : No debería haber ningun primario, si SV no ha recibido aún latidos
	t.Run("T1:soloArranqueYparada",
		func(t *testing.T) { cfg.soloArranqueYparadaTest1(t) })

	// Test2 : No debería haber ningun primario, si SV no ha recibido aún latidos
	t.Run("T2:ElegirPrimerLider",
		func(t *testing.T) { cfg.elegirPrimerLiderTest2(t) })

	// Test3: tenemos el primer primario correcto
	t.Run("T3:FalloAnteriorElegirNuevoLider",
		func(t *testing.T) { cfg.falloAnteriorElegirNuevoLiderTest3(t) })

	// Test4: Tres operaciones comprometidas en configuración estable
	t.Run("T4:tresOperacionesComprometidasEstable",
		func(t *testing.T) { cfg.tresOperacionesComprometidasEstable(t) })
}

// TEST primer rango
func TestAcuerdosConFallos(t *testing.T) { // (m *testing.M) {
	// <setup code>
	// Crear canal de resultados de ejecuciones ssh en maquinas remotas
	cfg := makeCfgDespliegue(t,
		3,
		[]string{REPLICA1, REPLICA2, REPLICA3},
		[]bool{true, true, true})

	// tear down code
	// eliminar procesos en máquinas remotas
	defer cfg.stop()

	// Test5: Se consigue acuerdo a pesar de desconexiones de seguidor
	t.Run("T5:AcuerdoAPesarDeDesconexionesDeSeguidor ",
		func(t *testing.T) { cfg.AcuerdoApesarDeSeguidor(t) })

	t.Run("T6:SinAcuerdoPorFallos ",
		func(t *testing.T) { cfg.SinAcuerdoPorFallos(t) })

	t.Run("T7:SometerConcurrentementeOperaciones ",
		func(t *testing.T) { cfg.SometerConcurrentementeOperaciones(t) })

}

// ---------------------------------------------------------------------
//
// Canal de resultados de ejecución de comandos ssh remotos
type canalResultados chan string

func (cr canalResultados) stop() {
	close(cr)

	// Leer las salidas obtenidos de los comandos ssh ejecutados
	for s := range cr {
		fmt.Println(s)
	}
}

// ---------------------------------------------------------------------
// Operativa en configuracion de despliegue y pruebas asociadas
type configDespliegue struct {
	t           *testing.T
	conectados  []bool
	numReplicas int
	barrier     string
	nodosRaft   []rpctimeout.HostPort
	idLider     int
	cr          canalResultados
}

// Crear una configuracion de despliegue
func makeCfgDespliegue(t *testing.T, n int, nodosraft []string,
	conectados []bool) *configDespliegue {
	cfg := &configDespliegue{}
	cfg.t = t
	cfg.conectados = conectados
	cfg.numReplicas = n
	cfg.nodosRaft = rpctimeout.StringArrayToHostPortArray(nodosraft)
	cfg.idLider = 0
	cfg.cr = make(canalResultados, 2000)

	return cfg
}

func (cfg *configDespliegue) stop() {
	//cfg.stopDistributedProcesses()

	time.Sleep(50 * time.Millisecond)

	cfg.cr.stop()
}

// --------------------------------------------------------------------------
// FUNCIONES DE SUBTESTS

// Se pone en marcha una replica ?? - 3 NODOS RAFT
func (cfg *configDespliegue) soloArranqueYparadaTest1(t *testing.T) {
	//t.Skip("SKIPPED soloArranqueYparadaTest1")

	fmt.Println(t.Name(), ".....................")

	cfg.t = t // Actualizar la estructura de datos de tests para errores

	// Poner en marcha replicas en remoto con un tiempo de espera incluido
	cfg.startDistributedProcesses()

	time.Sleep(3 * time.Second)

	// Comprobar estado replica 0
	cfg.comprobarEstadoRemoto(0, 0, false, -1)

	// Comprobar estado replica 1
	cfg.comprobarEstadoRemoto(1, 0, false, -1)

	// Comprobar estado replica 2
	cfg.comprobarEstadoRemoto(2, 0, false, -1)

	// Parar réplicas almacenamiento en remoto
	cfg.stopDistributedProcesses()

	fmt.Println(".............", t.Name(), "Superado")
}

// Primer lider en marcha - 3 NODOS RAFT
func (cfg *configDespliegue) elegirPrimerLiderTest2(t *testing.T) {
	//t.Skip("SKIPPED ElegirPrimerLiderTest2")

	fmt.Println(t.Name(), ".....................")

	cfg.startDistributedProcesses()

	time.Sleep(10 * time.Second)

	// Se ha elegido lider ?
	lider := cfg.pruebaUnLider(3)
	fmt.Printf("Lider inicial es %d\n", lider)

	// Parar réplicas alamcenamiento en remoto
	cfg.stopDistributedProcesses() // Parametros

	fmt.Println(".............", t.Name(), "Superado")
}

// Fallo de un primer lider y reeleccion de uno nuevo - 3 NODOS RAFT
func (cfg *configDespliegue) falloAnteriorElegirNuevoLiderTest3(t *testing.T) {
	//t.Skip("SKIPPED FalloAnteriorElegirNuevoLiderTest3")

	fmt.Println(t.Name(), ".....................")

	cfg.startDistributedProcesses()

	time.Sleep(10 * time.Second)

	lider := cfg.pruebaUnLider(3)
	fmt.Printf("Lider inicial es %d\n", lider)

	// Desconectar lider
	cfg.paroLeader()
	cfg.restartDistributedProcesses()
	// ???

	time.Sleep(10 * time.Second)

	lider = cfg.pruebaUnLider(3)
	fmt.Printf("Lider actual es %d\n", lider)

	// Parar réplicas almacenamiento en remoto
	cfg.stopDistributedProcesses() //parametros

	fmt.Println(".............", t.Name(), "Superado")
}

// 3 operaciones comprometidas con situacion estable y sin fallos - 3 NODOS RAFT
func (cfg *configDespliegue) tresOperacionesComprometidasEstable(t *testing.T) {
	//t.Skip("SKIPPED tresOperacionesComprometidasEstable")

	fmt.Println(t.Name(), ".....................")

	cfg.startDistributedProcesses()

	time.Sleep(10 * time.Second)

	lider := cfg.pruebaUnLider(3)
	fmt.Printf("Lider inicial es %d\n", lider)

	fmt.Printf("Comprometo entrada 1\n")
	cfg.someterOperacionYComprobarCompromiso(0, "escribir", "clave1", "valor1", "Operacion realizada con exito")
	fmt.Printf("Comprometo entrada 2\n")
	cfg.someterOperacionYComprobarCompromiso(1, "escribir", "clave2", "valor2", "Operacion realizada con exito")
	fmt.Printf("Comprometo entrada 3\n")
	cfg.someterOperacionYComprobarCompromiso(2, "leer", "clave1", "", "valor1")

	cfg.stopDistributedProcesses() //parametros

	fmt.Println(".............", t.Name(), "Superado")

	// A completar ???
}

// Se consigue acuerdo a pesar de desconexiones de seguidor -- 3 NODOS RAFT
func (cfg *configDespliegue) AcuerdoApesarDeSeguidor(t *testing.T) {
	//t.Skip("SKIPPED AcuerdoApesarDeSeguidor")

	// A completar ???

	// Comprometer una entrada

	cfg.t = t // Actualizar la estructura de datos de tests para errores

	//  Obtener un lider y, a continuación desconectar una de los nodos Raft

	fmt.Println(t.Name(), ".....................")

	cfg.startDistributedProcesses()

	time.Sleep(10 * time.Second)

	lider := cfg.pruebaUnLider(3)
	fmt.Printf("Lider inicial es %d\n", lider)

	fmt.Printf("Comprometo entrada 1\n")
	cfg.someterOperacionYComprobarCompromiso(0, "escribir", "clave3", "valor3", "Operacion realizada con exito")

	fmt.Println("Tirando nodo aleatorio")
	cfg.paroNodoRandom()
	time.Sleep(2 * time.Second)
	lider = cfg.pruebaUnLider(3)
	fmt.Printf("Lider actual es %d\n", lider)

	fmt.Printf("Comprometo entrada 2\n")
	cfg.someterOperacionYComprobarCompromiso(1, "escribir", "clave4", "valor4", "Operacion realizada con exito")
	fmt.Printf("Comprometo entrada 3\n")
	cfg.someterOperacionYComprobarCompromiso(2, "escribir", "clave3", "valor33", "Operacion realizada con exito")
	fmt.Printf("Comprometo entrada 4\n")
	cfg.someterOperacionYComprobarCompromiso(3, "leer", "clave4", "", "valor4")

	fmt.Printf("Reconecto nodos tirados\n")
	cfg.restartDistributedProcesses()
	time.Sleep(10 * time.Second)
	fmt.Printf("Comprometo entrada 5\n")
	cfg.someterOperacionYComprobarCompromiso(4, "escribir", "clave5", "valor5", "Operacion realizada con exito")
	fmt.Printf("Comprometo entrada 6\n")
	cfg.someterOperacionYComprobarCompromiso(5, "leer", "clave3", "", "valor33")
	fmt.Printf("Comprometo entrada 7\n")
	cfg.someterOperacionYComprobarCompromiso(6, "escribir", "clave5", "valor55", "Operacion realizada con exito")
	fmt.Printf("Comprometo entrada 8\n")
	cfg.someterOperacionYComprobarCompromiso(7, "leer", "clave5", "", "valor55")

	fmt.Printf("Todo correcto\n")

	cfg.stopDistributedProcesses() //parametros

	fmt.Println(".............", t.Name(), "Superado")

	// Comprobar varios acuerdos con una réplica desconectada

	// reconectar nodo Raft previamente desconectado y comprobar varios acuerdos
}

// NO se consigue acuerdo al desconectarse mayoría de seguidores -- 3 NODOS RAFT
func (cfg *configDespliegue) SinAcuerdoPorFallos(t *testing.T) {
	//	t.Skip("SKIPPED SinAcuerdoPorFallos")

	// A completar ???

	fmt.Println(t.Name(), ".....................")

	cfg.startDistributedProcesses()

	time.Sleep(10 * time.Second)

	lider := cfg.pruebaUnLider(3)
	fmt.Printf("Lider inicial es %d\n", lider)
	// Comprometer una entrada
	fmt.Printf("Comprometo entrada 1\n")
	cfg.someterOperacionYComprobarCompromiso(0, "escribir", "clave10", "10", "Operacion realizada con exito")

	//  Obtener un lider y, a continuación desconectar 2 de los nodos Raft
	fmt.Printf("Tirando procesos followers\n")
	cfg.paroFollowers()

	fmt.Printf("Comprometo entrada 2 y espero que falle\n")
	cfg.someterOperacionYComprobarFallo("escribir", "clave 11", "11")
	fmt.Printf("Comprometo entrada 3 y espero que falle\n")
	cfg.someterOperacionYComprobarFallo("escribir", "clave12", "12")
	fmt.Printf("Comprometo entrada 4 y espero que falle\n")
	cfg.someterOperacionYComprobarFallo("lectura", "clave11", "")

	fmt.Printf("Reconectando seguidores parados\n")
	cfg.restartDistributedProcesses()
	time.Sleep(10 * time.Second)

	fmt.Printf("Comprometo entrada 5\n")
	cfg.someterOperacionYComprobarCompromiso(4, "escribir", "clave12", "12", "Operacion realizada con exito")
	fmt.Printf("Comprometo entrada 6\n")
	cfg.someterOperacionYComprobarCompromiso(5, "escribir", "clave11", "11", "Operacion realizada con exito")
	fmt.Printf("Comprometo entrada 7\n")
	cfg.someterOperacionYComprobarCompromiso(6, "leer", "clave10", "", "10")

	fmt.Printf("Todo correcto\n")

	cfg.stopDistributedProcesses() //parametros

	fmt.Println(".............", t.Name(), "Superado")

	// Comprobar varios acuerdos con 2 réplicas desconectada

	// reconectar lo2 nodos Raft  desconectados y probar varios acuerdos
}

// Se somete 5 operaciones de forma concurrente -- 3 NODOS RAFT
func (cfg *configDespliegue) SometerConcurrentementeOperaciones(t *testing.T) {
	//t.Skip("SKIPPED SometerConcurrentementeOperaciones")

	// A completar ???

	// un bucle para estabilizar la ejecucion

	// Obtener un lider y, a continuación someter una operacion

	fmt.Println(t.Name(), ".....................")

	cfg.startDistributedProcesses()

	time.Sleep(10 * time.Second)

	lider := cfg.pruebaUnLider(3)
	fmt.Printf("Lider inicial es %d\n", lider)

	fmt.Printf("Comprometo entrada 1\n")
	cfg.someterOperacionYComprobarCompromiso(0, "escribir", "clave7", "7", "Operacion realizada con exito")

	// Someter 5 operaciones concurrentes
	go cfg.someterOperacionRemoto("escribir", "claveS", "S")
	go cfg.someterOperacionRemoto("lectura", "claveS", "")
	go cfg.someterOperacionRemoto("escribir", "claveF", "F")
	go cfg.someterOperacionRemoto("lectura", "claveF", "")
	go cfg.someterOperacionRemoto("escribir", "claveM", "M")

	time.Sleep(40 * time.Second) // Esperar a que se completen las operaciones
	// Comprobar estados de nodos Raft, sobre todo
	// el avance del mandato en curso e indice de registro de cada uno
	// que debe ser identico entre ellos
	cfg.comprobarEstadoRegistro(5)

	fmt.Printf("Todo correcto\n")

	cfg.stopDistributedProcesses()

	fmt.Println(".............", t.Name(), "Superado")
}

// --------------------------------------------------------------------------
// FUNCIONES DE APOYO
// Comprobar que hay un solo lider
// probar varias veces si se necesitan reelecciones
func (cfg *configDespliegue) pruebaUnLider(numreplicas int) int {
	for iters := 0; iters < 10; iters++ {
		time.Sleep(500 * time.Millisecond)
		mapaLideres := make(map[int][]int)
		for i := 0; i < numreplicas; i++ {
			if cfg.conectados[i] {
				if _, mandato, eslider, _ := cfg.obtenerEstadoRemoto(i); eslider {
					mapaLideres[mandato] = append(mapaLideres[mandato], i)
				}
			}
		}

		ultimoMandatoConLider := -1
		for mandato, lideres := range mapaLideres {
			if len(lideres) > 1 {
				cfg.t.Fatalf("mandato %d tiene %d (>1) lideres",
					mandato, len(lideres))
			}
			if mandato > ultimoMandatoConLider {
				ultimoMandatoConLider = mandato
			}
		}

		if len(mapaLideres) != 0 {

			cfg.idLider = mapaLideres[ultimoMandatoConLider][0]
			return mapaLideres[ultimoMandatoConLider][0] // Termina

		}
	}
	cfg.t.Fatalf("un lider esperado, ninguno obtenido")

	return -1 // Termina
}

func (cfg *configDespliegue) obtenerEstadoRemoto(
	indiceNodo int) (int, int, bool, int) {
	var reply raft.EstadoRemoto
	err := cfg.nodosRaft[indiceNodo].CallTimeout("NodoRaft.ObtenerEstadoNodo",
		raft.Vacio{}, &reply, 5000*time.Millisecond)
	check.CheckError(err, "Error en llamada RPC ObtenerEstadoRemoto")

	cfg.t.Log("Estado replica: ", reply.IdNodo, reply.Mandato, reply.EsLider, reply.IdLider, "\n")

	return reply.IdNodo, reply.Mandato, reply.EsLider, reply.IdLider
}

func (cfg *configDespliegue) obtenerEstadoRegistro(indiceNodo int) (int, int) {
	var reply raft.EstadoRegistro
	err := cfg.nodosRaft[indiceNodo].CallTimeout("NodoRaft.ObtenerEstadoRegistro", raft.Vacio{}, &reply, 5000*time.Millisecond)
	check.CheckError(err, "Error en la llamada RPC ObtenerEstadoRegistro")
	return reply.Index, reply.Term
}

// start  gestor de vistas; mapa de replicas y maquinas donde ubicarlos;
// y lista clientes (host:puerto)
func (cfg *configDespliegue) startDistributedProcesses() {
	//cfg.t.Log("Before starting following distributed processes: ", cfg.nodosRaft)

	for i, endPoint := range cfg.nodosRaft {
		despliegue.ExecMutipleHosts(EXECREPLICACMD+
			" "+strconv.Itoa(i)+" "+cfg.barrier+" "+
			rpctimeout.HostPortArrayToString(cfg.nodosRaft),
			[]string{endPoint.Host()}, cfg.cr, PRIVKEYFILE)

		cfg.conectados[i] = true
	}

	// aproximadamente 500 ms para cada arranque por ssh en portatil
	time.Sleep(10000 * time.Millisecond)
}

func (cfg *configDespliegue) restartDistributedProcesses() {
	//cfg.t.Log("Before starting following distributed processes: ", cfg.nodosRaft)

	for i, endPoint := range cfg.nodosRaft {
		if !cfg.conectados[i] {
			despliegue.ExecMutipleHosts(EXECREPLICACMD+
				" "+strconv.Itoa(i)+" "+cfg.barrier+" "+
				rpctimeout.HostPortArrayToString(cfg.nodosRaft),
				[]string{endPoint.Host()}, cfg.cr, PRIVKEYFILE)

			cfg.conectados[i] = true
		}
	}

	// aproximadamente 500 ms para cada arranque por ssh en portatil
	time.Sleep(10000 * time.Millisecond)
}

func (cfg *configDespliegue) stopDistributedProcesses() {
	var reply raft.Vacio

	for i, endPoint := range cfg.nodosRaft {
		if cfg.conectados[i] {
			err := endPoint.CallTimeout("NodoRaft.ParaNodo",
				raft.Vacio{}, &reply, 10*time.Millisecond)
			check.CheckError(err, "Error en llamada RPC Para nodo")
		}
	}
}

func (cfg *configDespliegue) paroLeader() {
	var reply raft.Vacio
	for i, endPoint := range cfg.nodosRaft {
		if i == cfg.idLider {
			err := endPoint.CallTimeout("NodoRaft.ParaNodo", raft.Vacio{}, &reply, 20*time.Millisecond)
			check.CheckError(err, "Error en la llamada RPC Para nodo")
			cfg.conectados[i] = false
		}
	}
}

func (cfg *configDespliegue) paroNodoRandom() {
	var reply raft.Vacio

	i := rand.Intn(3)

	err := cfg.nodosRaft[i].CallTimeout("NodoRaft.ParaNodo", raft.Vacio{}, &reply, 20*time.Millisecond)
	check.CheckError(err, "Error en la llamada RPC Para nodo")
	cfg.conectados[i] = false
	fmt.Printf("Nodo %d parado\n", i)
}

func (cfg *configDespliegue) paroFollowers() {
	var reply raft.Vacio
	for i, endPoint := range cfg.nodosRaft {
		if i != cfg.idLider {
			err := endPoint.CallTimeout("NodoRaft.ParaNodo", raft.Vacio{}, &reply, 20*time.Millisecond)
			check.CheckError(err, "Error en la llamada RPC Para nodo")
			cfg.conectados[i] = false
		}
	}
}

// Comprobar estado remoto de un nodo con respecto a un estado prefijado
func (cfg *configDespliegue) comprobarEstadoRemoto(idNodoDeseado int,
	mandatoDeseado int, esLiderDeseado bool, IdLiderDeseado int) {
	idNodo, mandato, esLider, idLider := cfg.obtenerEstadoRemoto(idNodoDeseado)

	//cfg.t.Log("Estado replica: ", idNodo, mandato, esLider, idLider, "\n")

	if idNodo != idNodoDeseado || mandato != mandatoDeseado ||
		esLider != esLiderDeseado || idLider != IdLiderDeseado {
		cfg.t.Fatalf("Estado incorrecto en replica %d en subtest %s",
			idNodoDeseado, cfg.t.Name())
	}

}

func (cfg *configDespliegue) someterOperacionRemoto(operacion string, clave string, valor string) (int, int, bool, int, string) {

	var reply raft.ResultadoRemoto
	fmt.Printf("Someter operacion a %d\n", cfg.idLider)
	err := cfg.nodosRaft[cfg.idLider].CallTimeout("NodoRaft.SometerOperacionRaft", raft.TipoOperacion{operacion, clave, valor}, &reply, 60000*time.Millisecond)
	check.CheckError(err, "Error en llamada RPC SometerOperacionRaft")

	cfg.idLider = reply.IdLider

	return reply.IndiceRegistro, reply.Mandato, reply.EsLider, reply.IdLider, reply.ValorADevolver
}

func (cfg *configDespliegue) someterOperacionYComprobarFallo(operacion string, clave string, valor string) {

	var reply raft.ResultadoRemoto
	fmt.Printf("Someter operacion a %d\n", cfg.idLider)
	err := cfg.nodosRaft[cfg.idLider].CallTimeout("NodoRaft.SometerOperacionRaft", raft.TipoOperacion{operacion, clave, valor}, &reply, 60000*time.Millisecond)
	//check.CheckError(err, "Error en llamada RPC SometerOperacionRaft")

	if err == nil {
		cfg.t.Fatalf("Acuerdo conseguido sin mayoria simple en subtest %s", cfg.t.Name())
	} else {
		fmt.Printf("Acuerdo no conseguido satisfactoriamente\n")
	}

}

func (cfg *configDespliegue) someterOperacionYComprobarCompromiso(indiceLog int, operacion string, clave string, valor string, valorDevuelto string) {

	indice, _, esLider, _, valorADevolver := cfg.someterOperacionRemoto(operacion, clave, valor)

	if indice != indiceLog || valorADevolver != valorDevuelto {
		cfg.t.Fatalf("Operacion no sometida correctamente en indice %d en subtest %s. %d:%d - %s:%s - %t", indiceLog, cfg.t.Name(), indice, indiceLog, valorADevolver, valorDevuelto, esLider)
	}
}

func (cfg *configDespliegue) comprobarEstadoRegistro(indiceLog int) {

	indices := make([]int, cfg.numReplicas)
	mandatos := make([]int, cfg.numReplicas)

	for i, _ := range cfg.nodosRaft {
		indices[i], mandatos[i] = cfg.obtenerEstadoRegistro(i)
	}

	fmt.Print(indices[0], " = ", indiceLog, "\n")

	if indices[0] != indiceLog {
		cfg.t.Fatalf("Avance de indice de registro incorrecto en subtest %s", cfg.t.Name())
	}

	for i := 1; i < cfg.numReplicas; i++ {
		fmt.Print(indices[0], " = ", indices[i], " - ", mandatos[0], " = ", mandatos[i], "\n")
		if indices[0] != indices[i] || mandatos[0] != mandatos[i] {
			cfg.t.Fatalf("No coincide el estado en todos los nodos en subtest %s", cfg.t.Name())
		}
	}
}
