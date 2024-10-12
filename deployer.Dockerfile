FROM ubuntu:20.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt update && \
    apt install -y software-properties-common && \
    apt-add-repository -y ppa:ondrej/php && apt update && \
    apt install -y --no-install-recommends ca-certificates git curl clang g++ make unzip locales openssl libssl-dev build-essential libxml2 libpq-dev libyaml-dev procps gawk autoconf automake bison libffi-dev libgdbm-dev libncurses5-dev libsqlite3-dev libtool pkg-config sqlite3 zlib1g-dev libreadline6-dev locales mlocate

SHELL ["/bin/bash", "-lc"]

RUN sed -i -e 's/# en_US.UTF-8 UTF-8/en_US.UTF-8 UTF-8/' /etc/locale.gen && \
    dpkg-reconfigure --frontend=noninteractive locales && \
    update-locale LANG=en_US.UTF-8

ENV LANG en_US.UTF-8 

# configure git a bit
RUN git config --global advice.detachedHead false && \
    git config --global init.defaultBranch main

ENV DEFAULT_RUBY_VERSION=3.1.6 \
    DEFAULT_NODE_VERSION=18.16.0 \
    DEFAULT_ERLANG_VERSION=26.2.5.2 \
    DEFAULT_ELIXIR_VERSION=1.16 \
    DEFAULT_BUN_VERSION=1.1.24 \
    DEFAULT_PHP_VERSION=8.1.0 \ 
    DEFAULT_PYTHON_VERSION=3.12

ARG NODE_BUILD_VERSION=5.3.8

# install a ruby to run the initial script
# RUN echo 'source "/etc/profile.d/rvm.sh"' >> ~/.bashrc && \
#     usermod -a -G rvm root && \
#     rvm install $DEFAULT_RUBY_VERSION && rvm --default use $DEFAULT_RUBY_VERSION && gem update --system && gem install bundler

RUN gpg --keyserver keyserver.ubuntu.com --recv-keys 409B6B1796C275462A1703113804BB82D39DC0E3 7D2BAF1CF37B13E2069D6956105BD0E739499BDB && \
    curl -sSL https://get.rvm.io | bash -s stable && \
    usermod -a -G rvm root && \
    source /etc/profile.d/rvm.sh && \
    rvm install $DEFAULT_RUBY_VERSION && rvm --default use $DEFAULT_RUBY_VERSION && gem update --system && gem install bundler && \
    echo -e "\nsource /etc/profile.d/rvm.sh" >> ~/.bash_profile && \
    echo -e "\nrvm use default &> /dev/null" >> ~/.bash_profile

# install mise
RUN curl https://mise.run | MISE_VERSION=v2024.8.6 sh && \
    echo -e "\n\nexport PATH=\"$HOME/.local/bin:$HOME/.local/share/mise/shims:$PATH\"" >> ~/.bash_profile

ENV MISE_PYTHON_COMPILE=false

# install asdf, its plugins and dependencies
RUN git clone https://github.com/asdf-vm/asdf.git $HOME/.asdf --branch v0.14.0 && \
    echo -e "\n\n## Configure ASDF \n. $HOME/.asdf/asdf.sh" >> ~/.bash_profile && \
    source $HOME/.asdf/asdf.sh && \
    # nodejs
    curl -L https://github.com/nodenv/node-build/archive/refs/tags/v$NODE_BUILD_VERSION.tar.gz -o node-build.tar.gz && \
    tar -xzf node-build.tar.gz && \
    env PREFIX=/usr/local ./node-build-$NODE_BUILD_VERSION/install.sh && \
    asdf plugin add nodejs https://github.com/asdf-vm/asdf-nodejs.git && \
    # elixir
    asdf plugin-add erlang https://github.com/michallepicki/asdf-erlang-prebuilt-ubuntu-20.04.git && \
    echo -e "local.hex\nlocal.rebar" > $HOME/.default-mix-commands && \
    asdf plugin add elixir https://github.com/asdf-vm/asdf-elixir.git && \
    asdf install erlang $DEFAULT_ERLANG_VERSION && asdf global erlang $DEFAULT_ERLANG_VERSION && \
    asdf install elixir $DEFAULT_ELIXIR_VERSION && asdf global elixir $DEFAULT_ELIXIR_VERSION && \
    # bun
    asdf plugin add bun https://github.com/cometkim/asdf-bun.git

ENV MIX_ENV=dev

COPY bin/flyctl /usr/local/bin/flyctl
COPY deploy.rb /deploy.rb
COPY deploy /deploy

RUN mkdir -p /usr/src/app

# need a login shell for rvm to work properly...
ENTRYPOINT ["/bin/bash", "--login", "-c"]
CMD ["/deploy.rb"]