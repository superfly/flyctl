{{ if .pinnedPythonVersion -}}
# TODO: Change to an officially released version of Python before deploying to production.
{{ end -}}
ARG PYTHON_VERSION={{ .pythonVersion }}-slim-bullseye

FROM python:${PYTHON_VERSION}

ENV PYTHONDONTWRITEBYTECODE 1
ENV PYTHONUNBUFFERED 1

{{ if .hasPostgres -}}
# install psycopg2 dependencies.
RUN apt-get update && apt-get install -y \                                                                                                                                    
    libpq-dev \                                                                                                                                                               
    gcc \                                                                                                                                                                     
    && rm -rf /var/lib/apt/lists/*                                                                                                                                            

{{ end -}}

RUN mkdir -p /code

WORKDIR /code

{{ if .pipenv -}}
RUN pip install pipenv
COPY Pipfile Pipfile.lock /code/
RUN pipenv install --deploy --system
{{ else if .poetry -}}
RUN pip install poetry
COPY pyproject.toml poetry.lock /code/
RUN poetry config virtualenvs.create false
RUN poetry install --only main --no-root --no-interaction
{{ else -}}
COPY requirements.txt /tmp/requirements.txt
RUN set -ex && \
    pip install --upgrade pip && \
    pip install -r /tmp/requirements.txt && \
    rm -rf /root/.cache/
{{ end -}}

COPY . /code
{{ if .collectStatic }}
{{ if not .hasRandomSecretKey -}}
ENV SECRET_KEY "{{ .randomSecretKey }}"
{{ end -}}
RUN python manage.py collectstatic --noinput
{{ end }}
EXPOSE 8000

{{ if .wsgiFound -}}
CMD ["gunicorn", "--bind", ":8000", "--workers", "2", "{{ .wsgiName }}.wsgi"]
{{ else -}}
# TODO: replace demo.wsgi with <project_name>.wsgi
CMD ["gunicorn", "--bind", ":8000", "--workers", "2", "demo.wsgi"]
{{ end -}}
