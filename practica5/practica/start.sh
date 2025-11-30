#!/bin/sh

# Añade el directorio actual al PATH del sistema.
# Esto permite ejecutar scripts locales sin escribir ./ delante.
export PATH=$PATH:$(pwd)

###############################################################################
# 1. CERRAR EL CLUSTER ACTUAL
###############################################################################
echo "Cerrar Cluster"
./close_cl.sh
# Ejecuta el script de limpieza:
# - borra el cluster kind
# - detiene y elimina contenedores previos
# Esto garantiza que siempre partimos de un entorno limpio.

###############################################################################
# 2. CREAR EL NUEVO CLUSTER KIND + REGISTRO LOCAL
###############################################################################
echo "Crear cluster"
./kind-with-registry.sh
# Crea:
# - un registro Docker local (si no existía)
# - un cluster kind con 1 control-plane y 4 workers
# - la configuración para que los nodos usen ese registro
# - un ConfigMap documentando el registro en Kubernetes

###############################################################################
# 3. COMPILAR Y CONSTRUIR EL SERVIDOR RAFT
###############################################################################
echo -e "\nCompilación del servidor"

# Elimina el binario previo del servidor para evitar usar versiones antiguas.
rm ./DockerFiles/servidor/srvraft >/dev/null 2>&1

cd ./raft
# Compilación del servidor en modo estático (necesario para contenedores minimalistas)
CGO_ENABLED=0 go build -o ./../DockerFiles/servidor/srvraft ./cmd/srvraft/main.go

# Construcción de la imagen Docker del servidor
cd ./../DockerFiles/servidor
docker build . -t localhost:5001/srvraft:latest
# Sube la imagen al registro local
docker push localhost:5001/srvraft:latest

cd ./../..   # vuelta al directorio raíz del proyecto

###############################################################################
# 4. COMPILAR Y CONSTRUIR EL CLIENTE RAFT
###############################################################################
echo -e "\nCompilación del cliente"

# Elimina el binario previo del cliente
rm ./DockerFiles/cliente/cltraft >/dev/null 2>&1

cd ./raft
# Compila el cliente (código en pkg/cltraft)
CGO_ENABLED=0 go build -o ./../DockerFiles/cliente/cltraft ./pkg/cltraft/cltraft.go

# Construye la imagen Docker del cliente
cd ./../DockerFiles/cliente
docker build . -t localhost:5001/cltraft:latest
# Sube la imagen al registro local
docker push localhost:5001/cltraft:latest

cd ./../..   # volver al directorio principal

###############################################################################
# 5. DESPLEGAR SERVIDOR Y CLIENTE EN KUBERNETES
###############################################################################
./go_pods.sh
# Reinicia completamente la ejecución en K8s:
# - borra pods/statefulsets/services previos
# - crea todo usando pods_go.yaml
