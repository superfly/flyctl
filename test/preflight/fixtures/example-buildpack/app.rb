require 'sinatra'

set :bind, '0.0.0.0'
set :port, 8080

get '/' do
  "Hello World!\n"
end
