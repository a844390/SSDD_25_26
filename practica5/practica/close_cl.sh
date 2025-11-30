# Añade el directorio actual al PATH, permitiendo ejecutar scripts o binarios
# que estén en este mismo directorio sin necesidad de poner ./ delante.
export PATH=$PATH:$(pwd)

# Elimina el cluster de Kubernetes creado con kind (incluye nodos y red virtual).
kind delete cluster

# Detiene los contenedores Docker que representan los nodos del cluster Kubernetes.
# Estos contenedores corresponden a los workers y al nodo de control.
docker stop kind-worker
docker stop kind-worker2
docker stop kind-worker3
docker stop kind-worker4
docker stop kind-control-plane

# Detiene el contenedor que actúa como registro Docker local.
docker stop kind-registry

# Elimina definitivamente los contenedores detenidos,
# liberando espacio en el sistema.
docker rm kind-registry
docker rm kind-control-plane
docker rm kind-worker
docker rm kind-worker2
docker rm kind-worker3
docker rm kind-worker4
