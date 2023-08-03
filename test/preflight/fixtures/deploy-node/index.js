const http = require('http');

http.createServer((request, response) => {
    response.writeHead(200,
        {
            'Content-Type': 'text/plain'
        }
    );

    // prints environment variable value
    response.write(`Hello, World! ${process.env["TEST_ID"]}\n`);
    response.end();

}).listen(8080);
