// AGENTV1 FILE START: isolated local TLS fixture only; never production telemetry.
const https=require('node:https');const fs=require('node:fs');
const summary={requests:0,accepted:0,points:0,hosts:[],metrics:[],secretInPayload:false};
https.createServer({key:fs.readFileSync('/tmp/fixture.key'),cert:fs.readFileSync('/tmp/fixture.crt'),minVersion:'TLSv1.2'},(req,res)=>{
 let chunks=[];let size=0;req.on('data',b=>{size+=b.length;if(size>4194304)req.destroy();else chunks.push(b)});req.on('end',()=>{
 summary.requests++;const raw=Buffer.concat(chunks).toString();summary.secretInPayload ||= raw.includes('package-test-key-not-real');
 if(req.url!='/api/v1/otlp/v1/metrics'||req.headers.authorization!=='ApiKey package-test-key-not-real'){res.writeHead(401);res.end();return}
 if(fs.existsSync('/tmp/fixture-unavailable')){res.writeHead(503,{'Retry-After':'1'});res.end();return}
 try{const data=JSON.parse(raw);for(const rm of data.resourceMetrics||[]){const attrs=Object.fromEntries((rm.resource?.attributes||[]).map(a=>[a.key,a.value.stringValue]));if(attrs['host.id'])summary.hosts=[...new Set([...summary.hosts,attrs['host.id']])];for(const sm of rm.scopeMetrics||[])for(const m of sm.metrics||[]){summary.metrics=[...new Set([...summary.metrics,m.name])];summary.points+=(m.gauge?.dataPoints||m.sum?.dataPoints||[]).length}}
 summary.accepted++;fs.writeFileSync('/tmp/fixture-summary.json',JSON.stringify(summary),{mode:0o600});res.writeHead(201,{'Content-Type':'application/json'});res.end('{"data":{"acceptedDataPoints":1}}')
 }catch{res.writeHead(422);res.end()}
 });
}).listen(8443,'127.0.0.1');
// AGENTV1 FILE END
