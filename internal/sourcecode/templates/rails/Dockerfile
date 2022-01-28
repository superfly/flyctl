# syntax = docker/dockerfile:experimental

ARG RUBY_VERSION=3.1.0-jemalloc
FROM quay.io/evl.ms/fullstaq-ruby:${RUBY_VERSION}-slim as build

ARG RAILS_ENV=production
ENV RAILS_ENV=${RAILS_ENV}

ARG RAILS_MASTER_KEY
ENV RAILS_MASTER_KEY=${RAILS_MASTER_KEY}

ENV PATH $PATH:/usr/local/bin

ENV BUNDLE_WITHOUT development:test
ENV BUNDLE_PATH vendor/bundle

RUN mkdir /app
WORKDIR /app

# Reinstall runtime dependencies that need to be installed as packages

RUN --mount=type=cache,id=apt-cache,sharing=locked,target=/var/cache/apt \
    --mount=type=cache,id=apt-lib,sharing=locked,target=/var/lib/apt \
    apt-get update -qq && \
    apt-get install --no-install-recommends -y \
    git build-essential libpq-dev wget vim curl gzip xz-utils \
    && rm -rf /var/lib/apt/lists /var/cache/apt/archives

RUN gem install -N bundler -v 2.2.32

# Install rubygems
COPY Gemfile* ./
RUN bundle install

COPY . .

RUN bundle exec rails assets:precompile

RUN rm -rf vendor/bundle/ruby/*/cache

FROM quay.io/evl.ms/fullstaq-ruby:${RUBY_VERSION}-slim

ARG RAILS_ENV=production
ENV RAILS_ENV=${RAILS_ENV}
ENV RAILS_SERVE_STATIC_FILES true
ENV BUNDLE_PATH vendor/bundle
ENV BUNDLE_WITHOUT development:test
ENV RAILS_MASTER_KEY=${RAILS_MASTER_KEY}

RUN --mount=type=cache,id=apt-cache,sharing=locked,target=/var/cache/apt \
    --mount=type=cache,id=apt-lib,sharing=locked,target=/var/lib/apt \
    apt-get update -qq && \
    apt-get install --no-install-recommends -y \
    postgresql-client file vim curl gzip \
    && rm -rf /var/lib/apt/lists /var/cache/apt/archives

COPY --from=build /app /app

WORKDIR /app

RUN mkdir -p tmp/pids

EXPOSE 8080

CMD ["bundle", "exec", "puma", "-C", "config/puma.rb"]
