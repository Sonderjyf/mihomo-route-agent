"""Disposable feasibility experiment, NOT a production Route Agent.

Uses only loopback ports, synthetic names and two isolated Mihomo processes.
Never edits the installed FlClash or Windows DNS configuration.
"""
import argparse
import concurrent.futures
import json
from pathlib import Path
import socket
import socketserver
import subprocess
import sys
import threading
import time
import urllib.request

sys.path.insert(0,str(Path('.research/tools').resolve()))
import dns.message
import dns.query
import dns.rrset
import yaml
from scripts.verify_jev import Client, load_key, state_for

ROOT=Path('.research/gate-lab').resolve()
EXE=Path('.research/bin/mihomo.exe').resolve()
CTRL='http://127.0.0.1:19091'
opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
events=[]
start=time.perf_counter()
def event(name,**kwargs):
    events.append({'event':name,'ms':round((time.perf_counter()-start)*1000,3),**kwargs})
def request(path,method='GET'):
    req=urllib.request.Request(CTRL+path,method=method)
    with opener.open(req,timeout=3) as r:
        raw=r.read()
        return json.loads(raw) if raw else r.status

class Provider(socketserver.ThreadingMixIn, __import__('http.server',fromlist=['HTTPServer']).HTTPServer):
    daemon_threads=True
class ProviderHandler(__import__('http.server',fromlist=['BaseHTTPRequestHandler']).BaseHTTPRequestHandler):
    def do_GET(self):
        body=(ROOT/'learned.yaml').read_bytes()
        self.send_response(200);self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body)
    def log_message(self,*args):pass

class GateHandler(socketserver.BaseRequestHandler):
    def handle(self):
        wire,sock=self.request
        q=dns.message.from_wire(wire)
        host=str(q.question[0].name).rstrip('.')
        event('gate_observed',host=host,qtype=q.question[0].rdtype)
        if self.server.block and host=='first.route-lab.test':
            with self.server.lock:
                if not self.server.committed:
                    event('decision_started',kind='jev_api' if self.server.client else 'controlled_stub')
                    self.server.decisions+=1
                    if self.server.client:
                        reply=self.server.client.call(state_for('proxy',host))
                        self.server.model_result=reply
                        decision=reply.get('accepted','UNCERTAIN')
                    else:
                        time.sleep(0.12)
                        decision='PROXY'
                    event('decision_completed',decision=decision)
                    if decision=='PROXY':
                        tmp=ROOT/'learned.tmp'
                        tmp.write_text('payload:\n  - DOMAIN,first.route-lab.test\n',encoding='utf-8')
                        tmp.replace(ROOT/'learned.yaml')
                        event('provider_published')
                        status=request('/providers/rules/learned-proxy','PUT')
                        meta=request('/providers/rules')['providers']['learned-proxy']
                        event('provider_ack',status=status,ruleCount=meta['ruleCount'])
                        assert status==204 and meta['ruleCount']==1
                    self.server.committed=True
        r=dns.message.make_response(q)
        if q.question[0].rdtype==1:
            r.answer=[dns.rrset.from_text(str(q.question[0].name),1,'IN','A','127.0.0.1')]
        elif q.question[0].rdtype==28:
            r.answer=[dns.rrset.from_text(str(q.question[0].name),1,'IN','AAAA','::1')]
        sock.sendto(r.to_wire(),self.client_address)
        event('gate_released',host=host,qtype=q.question[0].rdtype)

class UDPServer(socketserver.ThreadingUDPServer):
    daemon_threads=True
class HoldHandler(socketserver.BaseRequestHandler):
    def handle(self):
        self.request.settimeout(10)
        try:
            data=self.request.recv(100)
            self.request.sendall(data)
            self.request.recv(1)
        except OSError: pass
class TCPServer(socketserver.ThreadingTCPServer):
    daemon_threads=True

def launch(name,config):
    home=ROOT/name;home.mkdir(exist_ok=True)
    (home/'config.yaml').write_text(yaml.safe_dump(config,sort_keys=False),encoding='utf-8')
    log=(home/'core.log').open('w',encoding='utf-8')
    p=subprocess.Popen([str(EXE),'-d',str(home),'-f',str(home/'config.yaml')],stdout=log,stderr=log,creationflags=subprocess.CREATE_NO_WINDOW)
    p.lab_log=log
    return p

def query(name,qtype='A'):
    t=time.perf_counter()
    r=dns.query.udp(dns.message.make_query(name,qtype),'127.0.0.1',port=15354,timeout=3)
    return {'type':qtype,'elapsed_ms':round((time.perf_counter()-t)*1000,3),'answer':[str(x) for x in r.answer]}

def connect_socks():
    # A SOCKS hostname request proves provider ordering, independently of reverse-IP mapping.
    event('first_connection_started')
    s=socket.create_connection(('127.0.0.1',17891),timeout=3)
    s.sendall(b'\x05\x01\x00');assert s.recv(2)==b'\x05\x00'
    host=b'first.route-lab.test'
    s.sendall(b'\x05\x01\x00\x03'+bytes([len(host)])+host+(18081).to_bytes(2,'big'))
    reply=s.recv(256);assert reply[1]==0,reply
    s.sendall(b'route-lab-proof');assert s.recv(100)==b'route-lab-proof'
    event('first_connection_payload_verified')
    return s

def run(options):
    global ROOT,EXE
    ROOT=Path(options.workdir).resolve()
    EXE=Path(options.mihomo).resolve()
    if not EXE.is_file():
        raise FileNotFoundError('Set --mihomo to an existing official executable')
    ROOT.mkdir(parents=True,exist_ok=True)
    (ROOT/'learned.yaml').write_text('payload: []\n',encoding='utf-8')
    servers=[];processes=[]
    try:
        gate=UDPServer(('127.0.0.1',15355),GateHandler)
        gate.lock=threading.Lock();gate.committed=False;gate.block=False;gate.decisions=0
        gate.client=Client(load_key(options.key_file),options.proxy,timeout=2) if options.decision_backend=='jev' else None
        gate.model_result=None
        provider=Provider(('127.0.0.1',18765),ProviderHandler)
        target=TCPServer(('127.0.0.1',18081),HoldHandler)
        for s in [gate,provider,target]:
            servers.append(s);threading.Thread(target=s.serve_forever,daemon=True).start()
        hop=launch('hop',{'mixed-port':17892,'allow-lan':False,'bind-address':'127.0.0.1','mode':'rule','log-level':'error','rules':['MATCH,DIRECT'],'tun':{'enable':False},'hosts':{'first.route-lab.test':'127.0.0.1'}})
        processes.append(hop)
        base={'mixed-port':17891,'allow-lan':False,'bind-address':'127.0.0.1','external-controller':'127.0.0.1:19091',
              'mode':'rule','log-level':'debug','ipv6':True,'tun':{'enable':False},
              'dns':{'enable':True,'listen':'127.0.0.1:15354','ipv6':True,'enhanced-mode':'fake-ip',
                     'fake-ip-range':'198.19.0.1/16','use-hosts':False,'use-system-hosts':False,'nameserver':['udp://127.0.0.1:15355']},
              'proxies':[{'name':'lab-hop','type':'socks5','server':'127.0.0.1','port':17892}],
              'proxy-groups':[{'name':'PROXY','type':'select','proxies':['lab-hop']}],
              'rule-providers':{'learned-proxy':{'type':'http','behavior':'classical','format':'yaml','url':'http://127.0.0.1:18765/learned.yaml','path':'./learned.yaml','interval':3600}},
              'rules':['RULE-SET,learned-proxy,PROXY','MATCH,DIRECT']}
        main=launch('main',base);processes.append(main)
        for _ in range(50):
            try: request('/version');break
            except Exception:time.sleep(.1)
        before=len(events)
        fake=[query('fake.route-lab.test',t) for t in ['A','AAAA','HTTPS']]
        fake_gate_hits=len([e for e in events[before:] if e['event']=='gate_observed'])
        base['dns']['enhanced-mode']='redir-host'
        config_path=ROOT/'main/config.yaml'
        config_path.write_text(yaml.safe_dump(base,sort_keys=False),encoding='utf-8')
        # Only the isolated standalone core is reloaded.
        body=json.dumps({'path':str(config_path)}).encode()
        req=urllib.request.Request(CTRL+'/configs?force=true',data=body,method='PUT',headers={'Content-Type':'application/json'})
        with opener.open(req,timeout=5) as r:assert r.status==204
        gate.block=True
        with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
            real=list(pool.map(lambda t:query('first.route-lab.test',t),['A','AAAA','HTTPS']))
        event('client_received_all_dns')
        with connect_socks() as sock:
            time.sleep(.1)
            conns=request('/connections')['connections']
            matches=[{k:c.get(k) for k in ['rule','rulePayload','chains','metadata']} for c in conns if c.get('metadata',{}).get('destinationPort')=='18081']
            # Type varies across core versions.
            if not matches:matches=[{k:c.get(k) for k in ['rule','rulePayload','chains','metadata']} for c in conns if str(c.get('metadata',{}).get('destinationPort'))=='18081']
        result={'kind':'isolated_mechanics_not_live_flclash','decision_backend':options.decision_backend,'synthetic_evidence':True,'model_result':gate.model_result,'connection_identity':'SOCKS5 hostname, not TUN IP reverse mapping','fake_ip':fake,'fake_gate_hits':fake_gate_hits,'redir_host':real,'decision_count':gate.decisions,'connections':matches,'events':events}
        ack=next((e['ms'] for e in events if e['event']=='provider_ack'),None)
        release=next((e['ms'] for e in events if e['event']=='gate_released' and e['host']=='first.route-lab.test'),None)
        connect=next((e['ms'] for e in events if e['event']=='first_connection_started'),None)
        result['timing_order_passed']=None not in (ack,release,connect) and ack<release<connect
        result['passed']=fake_gate_hits==0 and gate.decisions==1 and result['timing_order_passed'] and any(c['rulePayload']=='learned-proxy' and 'lab-hop' in c['chains'] for c in matches)
        dest=Path(options.output);dest.parent.mkdir(parents=True,exist_ok=True)
        dest.write_text(json.dumps(result,indent=2),encoding='utf-8')
        print(json.dumps(result,indent=2))
        assert result['passed'], 'Inspect gate-experiment.json; the integration claim did not pass'
    finally:
        for p in processes:
            p.terminate()
            try:p.wait(timeout=5)
            except subprocess.TimeoutExpired:p.kill();p.wait()
            p.lab_log.close()
        for s in servers:s.shutdown();s.server_close()
        if 'gate' in locals() and gate.client:gate.client.close()

if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--mihomo',default='.research/bin/mihomo.exe')
    parser.add_argument('--workdir',default='.research/gate-lab')
    parser.add_argument('--decision-backend',choices=['stub','jev'],default='stub')
    parser.add_argument('--key-file')
    parser.add_argument('--proxy')
    parser.add_argument('--output',default='local-evidence/gate-experiment.json')
    run(parser.parse_args())
