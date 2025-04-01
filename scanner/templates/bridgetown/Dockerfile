ARG RUBY_VERSION={{ .rubyVersion }}
FROM ruby:$RUBY_VERSION-slim as base

ENV VOLTA_HOME=/usr/local

RUN apt-get update &&\
    apt-get install --yes build-essential git curl

RUN curl https://get.volta.sh | bash &&\
    volta install node@lts yarn@latest

WORKDIR /app

FROM base as gems
COPY Gemfile* .
RUN bundle install

FROM base
COPY . .
COPY --from=base $VOLTA_HOME/bin $VOLTA_HOME/bin
COPY --from=base $VOLTA_HOME/tools $VOLTA_HOME/tools
COPY --from=base /app /app
COPY --from=gems /usr/local/bundle /usr/local/bundle

RUN yarn install
RUN bundle exec bridgetown frontend:build

EXPOSE 4000
CMD bundle exec bridgetown start
