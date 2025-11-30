# Elimina el Pod llamado "cliente" si existe.
# Esto asegura que no haya una ejecución anterior interfiriendo.
kubectl delete pod cliente

# Elimina el Service llamado "raft-service".
# Este servicio es normalmente el responsable del descubrimiento o de exponer los Pods.
kubectl delete service raft-service

# Elimina el StatefulSet llamado "raft".
# También elimina los Pods asociados (raft-0, raft-1, raft-2…) y sus volúmenes si no son persistentes.
kubectl delete statefulset raft


echo "--------- Esperar un poco para dar tiempo que terminen Pods previos"

# Espera 1 segundo. Es una pausa corta para evitar que el siguiente comando
# se ejecute antes de que Kubernetes haya limpiado por completo los recursos.
sleep 1

# Crea todo lo definido en el manifiesto pods_go.yaml.
# Este fichero incluye los Pods, Servicios u otros objetos necesarios
# para lanzar nuevamente el despliegue en limpio.
kubectl create -f pods_go.yaml
