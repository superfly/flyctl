FROM debian:bookworm

RUN echo "deb http://deb.debian.org/debian testing main contrib non-free non-free-firmware" | tee /etc/apt/sources.list.d/testing.list

RUN apt update && apt install -y --no-install-recommends ca-certificates git curl clang g++ make unzip locales openssl libssl-dev ruby ruby-dev ruby-bundler build-essential libxml2 libpq-dev libyaml-dev

# Erlang + Elixir
COPY --from=hexpm/elixir:1.17.2-erlang-27.0.1-debian-bookworm-20240722-slim /usr/local/bin/ /usr/local/bin
COPY --from=hexpm/elixir:1.17.2-erlang-27.0.1-debian-bookworm-20240722-slim /usr/local/lib/elixir/ /usr/local/lib/elixir
COPY --from=hexpm/elixir:1.17.2-erlang-27.0.1-debian-bookworm-20240722-slim /usr/local/lib/erlang/ /usr/local/lib/erlang
# Ensure you have everything compiled so fly launch works
RUN mix local.hex --force && mix local.rebar --force
ENV MIX_ENV=dev

# Node.js
COPY --from=node:22-bookworm /usr/local/bin/ /usr/local/bin
COPY --from=node:22-bookworm /usr/local/lib/node_modules/ /usr/local/lib/node_modules
COPY --from=node:22-bookworm /opt/yarn-v1.22.22/ /opt/yarn-v1.22.22
ENV NODE_ENV=development

COPY bin/flyctl /usr/local/bin/flyctl
COPY deploy.rb /deploy.rb

WORKDIR /usr/src/app

CMD ["/deploy.rb"]