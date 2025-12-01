package main

import (
	"fmt"
	"raft/internal/comun/check"
	"raft/internal/comun/rpctimeout"
	"raft/internal/raft"
	"strconv"
	"time"
)

/*
    Programa que actua como cliente automático.
*/
func main() {
	fmt.Println("Inicio cliente")
    // Dominio DNS del servicio Headless de Kubernetes.
	// Necesario porque IPs en Kubernetes cambian, pero los nombres DNS de pods del StatefulSet no.
	dns := "raft-service.default.svc.cluster.local"
    // Prefijo del StatefulSet: los Pods se llaman raft-0, raft-1, raft-2
	name := "raft"
    // Puerto RPC del servidor Raft (debe coincidir con EXPOSE/Listen del servidor)
	puerto := "6000"
	var direcciones []string
    
    // Construcción dinámica de las direcciones del cluster.
	// Ejemplo final:
	//   raft-0.raft-service.default.svc.cluster.local:6000
	//   raft-1.raft-service.default.svc.cluster.local:6000
	//   raft-2.raft-service.default.svc.cluster.local:6000
	for i := 0; i < 3; i++ {
		nodo := name + "-" + strconv.Itoa(i) + "." + dns + ":" + puerto
		direcciones = append(direcciones, nodo)
	}
    // Lista de hosts accesibles mediante RPC con timeout automático
	var nodos []rpctimeout.HostPort
	for _, endPoint := range direcciones {
		nodos = append(nodos, rpctimeout.HostPort(endPoint))
	}

	fmt.Println("Inicializacion hecha")
    
    // Espera breve para asegurarse de que el cluster Raft haya elegido líder tras arrancar.
	// (En Kubernetes el cliente puede levantarse antes que Raft, así que esto ayuda a evitar errores tempranos.)
	time.Sleep(10 * time.Second)
	// reply contendrá la respuesta del servidor: líder detectado, valor devuelto, etc.
	var reply raft.ResultadoRemoto

	// Se crean dos operaciones:
	// - operacion1: escribir la clave "clave1" = "valor1"
	// - operacion2: leer la clave "clave1"
	operacion1 := raft.TipoOperacion{"escribir", "clave1", "valor1"}
	operacion2 := raft.TipoOperacion{"leer", "clave1", ""}

	fmt.Println("Operacion 1 sometida a 0")

	// Primer intento: enviar operación al nodo 0.
	// Si este nodo NO es el líder, responderá indicando quién lo es.
	err := nodos[0].CallTimeout(
		"NodoRaft.SometerOperacionRaft",
		operacion1,
		&reply,
		5000*time.Millisecond,
	)
	check.CheckError(err, "SometerOperacion")

	// Mientras la respuesta no indique quién es el líder del cluster, seguimos reintentando.
	// reply.IdLider == -1 significa "no hay líder elegido aún".
	for reply.IdLider == -1 {
		err = nodos[0].CallTimeout(
			"NodoRaft.SometerOperacionRaft",
			operacion1,
			&reply,
			5000*time.Millisecond,
		)
		check.CheckError(err, "SometerOperacion")
	}

	// Si el nodo inicial no era el líder, reenviamos la operación al nodo indicado.
	if !reply.EsLider {
		fmt.Printf("Operacion 1 sometida a %d\n", reply.IdLider)

		err = nodos[reply.IdLider].CallTimeout(
			"NodoRaft.SometerOperacionRaft",
			operacion1,
			&reply,
			5000*time.Millisecond,
		)
		check.CheckError(err, "SometerOperacion")
	}

	// Imprime el resultado de la operación de escritura.
	fmt.Printf("Valor devuelto: %s\n", reply.ValorADevolver)

	// Ahora hacemos una operación de lectura GET contra el líder.
	fmt.Printf("Operacion 2 sometida a %d\n", reply.IdLider)

	err = nodos[reply.IdLider].CallTimeout(
		"NodoRaft.SometerOperacionRaft",
		operacion2,
		&reply,
		5000*time.Millisecond,
	)
	check.CheckError(err, "SometerOperacion")

	// Imprime resultado final
	fmt.Printf("Valor devuelto: %s\n", reply.ValorADevolver)
}
