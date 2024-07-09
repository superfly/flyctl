FROM python:{{ .pyVersion }} AS builder

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1

WORKDIR /app
COPY . .

{{ if .pipenv -}}

ENV PIPENV_VENV_IN_PROJECT=1 \
    PIPENV_CUSTOM_VENV_NAME=.venv
RUN pip install pipenv
RUN pipenv install

{{ else if .poetry -}}

RUN pip install poetry
RUN poetry config virtualenvs.in-project true
RUN poetry install

{{ else if .pep621 -}}

RUN python -m venv .venv && \
    . .venv/bin/activate && \
    pip install --upgrade pip && \
    pip install .

{{ else if .pip }}

RUN python -m venv .venv && \
    . .venv/bin/activate && \
    pip install --upgrade pip && \
    pip install -r requirements.txt

{{ end -}}

FROM python:{{ .pyVersion }}-slim

WORKDIR /app
COPY --from=builder /app .

{{ if .flask -}}
CMD ["/app/.venv/bin/flask", "run", "--host=0.0.0.0", "--port=8080"]
{{ else if .fastapi -}}
CMD ["/app/.venv/bin/fastapi", "run"]
{{ else if .streamlit -}}
CMD ["/app/.venv/bin/streamlit", "run", "{{ .entrypoint }}"]
{{ end -}}