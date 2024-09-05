#!/bin/sh
sleep 1; while [ ! -f /CONTAINER_READY ]; do sleep 1; done
exit 0
