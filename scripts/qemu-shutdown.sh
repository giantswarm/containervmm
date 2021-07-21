#!/bin/bash

if [ $# -eq 0 ]
then
    echo "usage: $0 <shutdown-deferrer url>"
    exit 1
fi

poll_interval=5
poll_timeout=120
shutdown_deferrer_url=$1

# Poll shutdown-deferrer service in order to wait for proper node draining
# before VM shutdown.
defer="true"
while [ "$defer" = "true" -a $poll_timeout -gt 0 ]
do
    sleep $poll_interval
    defer=$(/usr/bin/curl -qsS $shutdown_deferrer_url)
    echo "GET /v1/defer: $defer"
    poll_timeout=$(/bin/expr $poll_timeout - $poll_interval)
done

# Send SIGTERM signal to containervmm which will in turn send a graceful shutdown command to qemu monitor.
pkill containervmm

# Wait while VM shutting down (socket exists) and then return.
while [ -S /tmp/qmp-socket ]
do
  sleep 0.1
done

exit 0
