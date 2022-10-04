FROM heroku/heroku:20
RUN useradd -m heroku
RUN mkdir /app
WORKDIR /app
ENV HOME /app
ENV PORT 8080
COPY Procfile /app
COPY entrypoint.sh /app
ENTRYPOINT ["/bin/bash", "/app/entrypoint.sh"]

RUN curl "https://s3-external-1.amazonaws.com/heroku-slugs-us/d82f/d82f62eb-1127-4830-8f13-a08e8265b2e8.tar.gz?AWSAccessKeyId=AKIAZSXS6CXK4G6YZGNK&Signature=Ith73v1twB9f6W34i%2FmiL3cdSd0%3D&Expires=1664895515" | tar xzf - --strip 2 -C /app
RUN chown -R heroku:heroku /app

USER heroku
