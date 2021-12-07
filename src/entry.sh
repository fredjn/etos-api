#!/bin/bash

exec opentelemetry-instrument \
		uvicorn etos_api.main:APP \
			--host 0.0.0.0 \
			--port 8080
