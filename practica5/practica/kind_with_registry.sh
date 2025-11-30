#!/bin/sh
# Usamos /bin/sh como intérprete del script.

set -o errexit
# Hace que el script termine inmediatamente si cualquier comando falla.
# Esto evita que el cluster quede en un estado inconsistente.

###############################################################################
# 1. CREAR REGISTRO DOCKER LOCAL SI NO EXISTE
###############################################################################

# Nombre del contenedor que actuará como registro local de Docker.
reg_name='kind-registry'
# Puerto local por el que se expondrá el registro (lo verás como localhost:5001).
reg_port='5001'

# Comprueba si el contenedor del registro YA está corriendo.
# Si la inspección devuelve "true", ya está arrancado.
# Si no es "true", crea y arranca el registro.
if [ "$(docker inspect -f '{{.State.Running}}' "${reg_name}" 2>/dev/null || true)" != 'true' ]; then
  docker run \
    -d \                             # Ejecuta el contenedor en segundo plano
    --restart=always \              # Se reiniciará automáticamente si se detiene
    -p "127.0.0.1:${reg_port}:5000" \   # Mapea puerto local 5001 → puerto 5000 del registro
    --name "${reg_name}" \          # Nombre del contenedor
    registry:2                      # Imagen oficial del registro Docker
fi

###############################################################################
# 2. CREAR CLUSTER KIND CON REGISTRO LOCAL CONFIGURADO
###############################################################################

# Genera dinámicamente la configuración del cluster usando un heredoc.
# La entrada se pasa directamente a 'kind create cluster' por stdin ("--config=-").
cat <<EOF | kind create cluster --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4

# Lista de nodos que formarán el cluster Kubernetes.
nodes:
- role: control-plane    # Nodo maestro
- role: worker           # Worker 1
- role: worker           # Worker 2
- role: worker           # Worker 3
- role: worker           # Worker 4

# Parches que se aplican a containerd (el runtime de los nodos).
# Esto permite que los nodos de Kubernetes usen el registro local.
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${reg_port}"]
    endpoint = ["http://${reg_name}:5000"]
  # Esto indica a containerd que cuando se use una imagen con host localhost:5001
  # debe buscarla realmente en el contenedor "kind-registry" usando su puerto 5000.

EOF
# Fin del heredoc enviado a kind.

###############################################################################
# 3. CONECTAR EL REGISTRO A LA RED DEL CLUSTER KIND
###############################################################################

# Si el contenedor del registro NO está conectado a la red 'kind', lo conectamos.
# Los nodos Kubernetes están dentro de esa red, y deben poder acceder al registro.
if [ "$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' "${reg_name}")" = 'null' ]; then
  docker network connect "kind" "${reg_name}"
fi

###############################################################################
# 4. DOCUMENTAR EL REGISTRO LOCAL EN EL CLUSTER (ConfigMap)
###############################################################################

# Aplica un ConfigMap estándar que informa al cluster de que tiene un registro local.
# Lo coloca en kube-public para que sea visible por cualquiera.
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting        # Nombre del ConfigMap
  namespace: kube-public              # Namespace público
data:
  localRegistryHosting.v1: |
    host: "localhost:${reg_port}"     # Indica que el registro está en localhost:5001
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
