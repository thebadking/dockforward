const express = require('express');
const app = express();
const port = 3000;

app.get('/', (req, res) => {
  res.json({
    message: 'Hello from remote-docker test service!',
    timestamp: new Date().toISOString()
  });
});

app.listen(port, '0.0.0.0', () => {
  console.log(`Test service listening at http://0.0.0.0:${port}`);
});
