FROM pierrezemb/gostatic
{{ if .contentDir -}}
COPY {{ .contentDir }} /srv/http/
{{ else -}}
COPY . /srv/http/
{{ end -}}
CMD ["-port","8080","-https-promote", "-enable-logging"]
