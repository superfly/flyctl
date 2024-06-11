FROM python:{{ .pyVersion }} AS builder

ENV PYTHONUNBUFFERED=1 \ 
    PYTHONDONTWRITEBYTECODE=1 

RUN pip install poetry && poetry config virtualenvs.in-project true

WORKDIR /app

COPY pyproject.toml poetry.lock ./
COPY {{ .appName }}/ ./{{ .appName }}

RUN poetry install

FROM python:{{ .pyVersion }}-slim

WORKDIR /app

COPY --from=builder /app .
# Copy additional resources for your app here
# COPY templates/ templates
# COPY static/ static


CMD ["/app/.venv/bin/fastapi", "run", "{{ .appFile }}"]