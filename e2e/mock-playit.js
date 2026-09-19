// Mock playit.gg API for end-to-end testing of the DiscoPanel playit module
// watcher. Implements POST /tunnels/list and POST /tunnels/create. Tunnels
// created via the API get a display address ~2 seconds later (simulating the
// async allocation of the real service).
const http = require('http');
const crypto = require('crypto');

const PORT = 8901;
const tunnels = [];

function readBody(req) {
  return new Promise((resolve) => {
    let data = '';
    req.on('data', (c) => (data += c));
    req.on('end', () => resolve(data));
  });
}

const server = http.createServer(async (req, res) => {
  const auth = req.headers['authorization'] || '(none)';
  console.log(`[mock-playit] ${req.method} ${req.url} auth=${auth}`);

  if (req.url === '/tunnels/list' && req.method === 'POST') {
    await readBody(req);
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ data: { tunnels } }));
    return;
  }

  if (req.url === '/tunnels/create' && req.method === 'POST') {
    const body = JSON.parse((await readBody(req)) || '{}');
    const id = 'tun-' + crypto.randomUUID().slice(0, 8);
    const localPort = body.origin?.data?.local_port || 25565;
    const tunnel = {
      id,
      name: body.name || 'mock-tunnel',
      tunnel_type: body.tunnel_type || 'minecraft-java',
      port_type: body.port_type || 'tcp',
      display_address: '',
      alloc: { data: { port_start: 0 } },
      origin: { type: 'default', data: { local_ip: '127.0.0.1', local_port: localPort } },
    };
    tunnels.push(tunnel);
    console.log(`[mock-playit] created tunnel ${id} name=${tunnel.name} local_port=${localPort}`);
    // Simulate async allocation by playit's fabric
    setTimeout(() => {
      tunnel.display_address = 'e2e-playit-test.ply.gg';
      tunnel.alloc.data.port_start = 54123;
      console.log(`[mock-playit] allocated ${tunnel.display_address}:${tunnel.alloc.data.port_start} for ${id}`);
    }, 2000);
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ data: { tunnel_id: id } }));
    return;
  }

  res.writeHead(404, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify({ error: 'not found' }));
});

server.listen(PORT, () => console.log(`[mock-playit] listening on http://localhost:${PORT}`));
