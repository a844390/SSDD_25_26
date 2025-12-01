# Práctica 5 — Kubernetes + Raft

---

## Descripción del proyecto

Este proyecto implementa un sistema distribuido de almacenamiento **clave/valor basado en Raft**, desplegado en un clúster **Kubernetes** utilizando **kind**.

El entorno permite **tolerancia a fallos**: si una réplica del servidor cae, Kubernetes la recrea automáticamente y Raft transfiere su estado al nodo recuperado, restaurando el registro y la máquina de estados.

---

## Objetivos alcanzados

- ✔ Ejecución del servicio Raft en Kubernetes  
- ✔ Recuperación automática de nodos caídos  
- ✔ Descubrimiento de nodos mediante servicios y DNS interno  
- ✔ Compilación estática y contenedores ligeros (`scratch` y `alpine`)  
- ✔ Automatización completa del despliegue mediante scripts  
- ✔ Cliente ejecutado dentro del clúster para pruebas internas  

---

## Estructura del proyecto

| Archivo / Carpeta | Tipo | Descripción |
|-------------------|------|-------------|
| `start.sh` | Script | Script principal del proyecto. Genera el clúster, compila el código, construye y sube las imágenes Docker y despliega la infraestructura en Kubernetes. |
| `go_pods.sh` | Script | Reinicia la ejecución eliminando pods anteriores y creando los nuevos definidos en `pods_go.yaml`. |
| `kind-with-registry.sh` | Script | Crea un clúster **kind** con su propio registro Docker accesible por los nodos del clúster. |
| `close_cl.sh` | Script | Limpia completamente el entorno eliminando clúster, contenedores y el registro local. |
| `pods_go.yaml` | Manifiesto K8s | Define los Pods y servicio necesario para ejecutar el servidor Raft y el cliente dentro del clúster. |
| `DockerFiles/cliente/` | Carpeta | Contiene el Dockerfile del cliente Raft y el binario compilado. |
| `DockerFiles/servidor/` | Carpeta | Contiene el Dockerfile del servidor Raft y el binario `srvraft`. |
| `raft/` | Carpeta | Código fuente del sistema Raft (servidor, cliente, RPC, estructura, registros, persistencia, etc.). |
| `srvraft` / `cltraft` | Binarios | Ejecutables generados automáticamente por `start.sh` para construir imágenes Docker. |

---

## Dockerfiles

```dockerfile
##################################################
# Cliente (basado en Alpine)
##################################################
FROM alpine

COPY cltraft /usr/local/bin/cliente
RUN chmod +x /usr/local/bin/cliente

EXPOSE 7000
```
```dockerfile
##################################################
# Servidor (basado en scratch)
##################################################
FROM scratch

COPY srvraft /usr/local/bin/srvraft

EXPOSE 6000
```

### Despliegue automático
Ejecutanto ./start.sh
- Compila Go (Cliente y servidor)
- Construye las imágenes de Docker
- Subida al registro local
- Crea los cúster Kubernetes
- Ejecuta pods y servicio

### Comados para conocer el estado del clúster
- kubectl get pods
- kubectl get svc

### Logs del cliente
- kubectl logs cliente

### Comprobación de recuperación automática
- kubectl delete pod raft-0

### Comandos útiles de docker
```script
- docker ps #para ver los contenedores en ejecución
```

Docker (host)

   --> Kind cluster (5 nodos → cada uno es un contenedor Docker)
   
   --> Kubernetes Pods (tus contenedores Raft y Cliente)
