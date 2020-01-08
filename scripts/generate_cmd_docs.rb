require 'yaml'
require 'erb'
require 'open3'

infile = ARGV.first
outfile = ARGV.last

data = YAML.load_file(infile)

def sanitize(data)
  data.gsub(/"/, "\\\"").gsub(/\n/, "\\n").chomp
end

template = <<-go
package docstrings

func Get(key string) KeyStrings {
  switch key {
  <% data.each do |key, cmd| %>
      case "<%= key %>":
        return KeyStrings{
          Usage: "<%= sanitize(cmd["usage"]) %>", 
          Short: "<%= sanitize(cmd["short"]) %>",
          Long: "<%= sanitize(cmd["long"]) %>",
        };
  <% end -%>
  }
  panic("cmd not found: " + key);
}
go

b = binding

out = ERB.new(template, trim_mode: "-").result(b)

stdout, status = Open3.capture2("gofmt", stdin_data: out)

unless status.success?
  puts "Error running gofmt: #{status}\n#{stdout}"
  exit status.to_i
end

File.open(outfile, "w") do |f|
  f.write(stdout)
end

puts %(Generated command docs "#{infile}" to #{outfile})