FROM ubuntu:focal

# RUN echo "deb http://deb.debian.org/debian testing main contrib non-free non-free-firmware" | tee /etc/apt/sources.list.d/testing.list

ENV DEBIAN_FRONTEND=noninteractive

RUN apt update && apt install -y --no-install-recommends software-properties-common && \
    apt-add-repository -y ppa:rael-gc/rvm && apt update && apt install -y --no-install-recommends ca-certificates git curl clang g++ make unzip locales openssl libssl-dev rvm build-essential libxml2 libpq-dev libyaml-dev procps gawk autoconf automake bison libffi-dev libgdbm-dev libncurses5-dev libsqlite3-dev libtool pkg-config sqlite3 zlib1g-dev libreadline6-dev software-properties-common

# install a ruby
RUN /bin/bash -lc 'rvm install 3.1.6 && rvm --default use 3.1.6 && gem update --system && gem install bundler'

# install asdf
RUN  git config --global advice.detachedHead false; \
     git clone https://github.com/asdf-vm/asdf.git $HOME/.asdf --branch v0.14.0 && \
     /bin/bash -c 'echo -e "\n\n## Configure ASDF \n. $HOME/.asdf/asdf.sh" >> ~/.bashrc' && \
     /bin/bash -c 'source ~/.asdf/asdf.sh; asdf plugin add nodejs https://github.com/asdf-vm/asdf-nodejs.git' && \
     /bin/bash -c 'source ~/.asdf/asdf.sh; asdf plugin add elixir https://github.com/asdf-vm/asdf-elixir.git' && \
     /bin/bash -c 'source ~/.asdf/asdf.sh; asdf plugin add php https://github.com/asdf-community/asdf-php.git' && \
     /bin/bash -c 'source ~/.asdf/asdf.sh; asdf plugin add bun https://github.com/cometkim/asdf-bun.git' && \
     /bin/bash -c 'source ~/.asdf/asdf.sh; asdf plugin add python https://github.com/danhper/asdf-python.git'

# # Erlang + Elixir
# COPY --from=hexpm/elixir:1.17.2-erlang-27.0.1-debian-bookworm-20240722-slim /usr/local/bin/ /usr/local/bin
# COPY --from=hexpm/elixir:1.17.2-erlang-27.0.1-debian-bookworm-20240722-slim /usr/local/lib/elixir/ /usr/local/lib/elixir
# COPY --from=hexpm/elixir:1.17.2-erlang-27.0.1-debian-bookworm-20240722-slim /usr/local/lib/erlang/ /usr/local/lib/erlang
# # Ensure you have everything compiled so fly launch works
# RUN mix local.hex --force && mix local.rebar --force
# ENV MIX_ENV=dev

# # Node.js
# COPY --from=node:22-bookworm /usr/local/bin/ /usr/local/bin
# COPY --from=node:22-bookworm /usr/local/lib/node_modules/ /usr/local/lib/node_modules
# COPY --from=node:22-bookworm /opt/yarn-v1.22.22/ /opt/yarn-v1.22.22
# ENV NODE_ENV=development

COPY bin/flyctl /usr/local/bin/flyctl
COPY deploy.rb /deploy.rb

WORKDIR /usr/src/app

# need a login shell for rvm to work properly...
ENTRYPOINT ["/bin/bash", "-lc"]
CMD ["/deploy.rb"]