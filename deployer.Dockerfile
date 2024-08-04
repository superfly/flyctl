FROM debian:bookworm

RUN apt update && apt install -y --no-install-recommends ruby ruby-bundler git curl clang g++ libncurses5 libncurses-dev libncurses5-dev make unzip locales openssl libssl-dev

# Erlang + Elixir
COPY --from=hexpm/elixir:1.17.2-erlang-27.0.1-debian-bookworm-20240722-slim /usr/local/bin/ /usr/local/bin
COPY --from=hexpm/elixir:1.17.2-erlang-27.0.1-debian-bookworm-20240722-slim /usr/local/lib/elixir/ /usr/local/lib/elixir
COPY --from=hexpm/elixir:1.17.2-erlang-27.0.1-debian-bookworm-20240722-slim /usr/local/lib/erlang/ /usr/local/lib/erlang
# Ensure you have everything compiled so fly launch works
RUN mix local.hex --force && mix local.rebar --force
ENV MIX_ENV=prod

# Node.js
COPY --from=node:22-bookworm /usr/local/bin/ /usr/local/bin
COPY --from=node:22-bookworm /usr/local/lib/node_modules/ /usr/local/lib/node_modules
COPY --from=node:22-bookworm /opt/yarn-v1.22.22/ /opt/yarn-v1.22.22
ENV NODE_ENV=production

COPY bin/flyctl /usr/local/bin/flyctl
COPY deploy.rb /deploy.rb

WORKDIR /usr/src/app

CMD ["/deploy.rb"]