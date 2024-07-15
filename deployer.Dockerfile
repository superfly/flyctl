FROM debian:bookworm

RUN apt update && apt install -y --no-install-recommends ruby git

COPY bin/flyctl /usr/local/bin/flyctl
COPY deploy.rb /deploy.rb

WORKDIR /usr/src/app

CMD ["/deploy.rb"]