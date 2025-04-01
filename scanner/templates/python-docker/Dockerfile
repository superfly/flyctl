FROM python:{{ .pyVersion }} AS builder

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1
WORKDIR /app

{{ if .pipenv -}}
ENV PIPENV_VENV_IN_PROJECT=1 \
    PIPENV_CUSTOM_VENV_NAME=.venv
RUN pip install pipenv
COPY Pipfile Pipfile.lock ./
RUN pipenv install
{{ else if .poetry -}}
RUN pip install poetry
RUN poetry config virtualenvs.in-project true
COPY pyproject.toml poetry.lock ./
RUN poetry install
{{ else if .pep621 -}}
RUN python -m venv .venv
COPY pyproject.toml ./
RUN .venv/bin/pip install .
{{ else if .pip }}
RUN python -m venv .venv
COPY requirements.txt ./
RUN .venv/bin/pip install -r requirements.txt
{{ end -}}

FROM python:{{ .pyVersion }}-slim
WORKDIR /app
COPY --from=builder /app/.venv .venv/
COPY . .
{{ if .flask -}}
CMD ["/app/.venv/bin/flask", "run", "--host=0.0.0.0", "--port=8080"]
{{ else if .fastapi -}}
CMD ["/app/.venv/bin/fastapi", "run"]
{{ else if .streamlit -}}
CMD ["/app/.venv/bin/streamlit", "run", "{{ .entrypoint }}"]
{{ end -}}
