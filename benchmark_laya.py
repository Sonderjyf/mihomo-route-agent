"""Reproducible offline, one-checkpoint benchmark; no production network hooks."""
import os
os.environ.update(HF_HUB_OFFLINE='1',TRANSFORMERS_OFFLINE='1',USE_TF='0')
import concurrent.futures
import json
from pathlib import Path
import statistics
import sys
import threading
import time

began=time.perf_counter()
import psutil
import torch
import transformers
import laya

device=sys.argv[1]
torch.set_num_threads(4)
torch.set_num_interop_threads(1)
proc=psutil.Process()
measure={'device_requested':device,'torch':torch.__version__,'transformers':transformers.__version__,
         'laya':laya.__version__,'threads':4,'import_s':time.perf_counter()-began,'samples':100,
         'offline':True,'memory_before_mb':proc.memory_info().rss/2**20}
memory=[]
stop=threading.Event()
def sample_memory():
    while not stop.wait(.05):memory.append(proc.memory_info().rss/2**20)
threading.Thread(target=sample_memory,daemon=True).start()
t=time.perf_counter()
agent=laya.load(str(Path('.research/model').resolve()),device=device,backend='eager')
measure.update(load_s=time.perf_counter()-t,device_actual=str(agent.device),dtype=str(agent.dtype),
               ram_loaded_mb=proc.memory_info().rss/2**20,parameters=sum(p.numel() for p in agent.model.parameters()))
assert str(agent.device).startswith(device), 'Device fallback invalidates benchmark'
QUESTIONS={'route':{'type':'choice','instructions':'Choose a route based only on the supplied evidence. Missing evidence means UNCERTAIN. A domain name alone does not prove reachability.',
                    'criteria':{'DIRECT':'Evidence confirms direct access works.','PROXY':'Evidence confirms proxy is needed.','UNCERTAIN':'Insufficient or conflicting evidence.'}}}
def state(i):
    return {'hostname':f'api{i}.route-lab.test','registrable_domain':'route-lab.test','tld':'test',
            'rule_evidence':{'known_proxy':False,'known_direct':False,'cn':False,'gfw':False},'history':{'previous_decision':None}}
def infer(i):
    if device=='cuda':torch.cuda.synchronize()
    t=time.perf_counter()
    result=agent.predict(state(i),QUESTIONS)
    if device=='cuda':torch.cuda.synchronize()
    return (time.perf_counter()-t)*1000,result
measure['cold_ms'],cold=infer(0)
print(json.dumps({'loaded':measure,'first_result':cold}),flush=True)
for i in range(5):infer(i)
times=[];counts={}
for i in range(100):
    elapsed,result=infer(i)
    times.append(elapsed)
    choice=result['answers']['route']['choice']
    counts[choice]=counts.get(choice,0)+1
    if i in [24,49,74]:print('completed',i+1,device,flush=True)
ordered=sorted(times)
measure.update(p50_ms=statistics.median(times),p95_ms=ordered[94],mean_ms=statistics.mean(times),max_ms=max(times),choices=counts)
# Four client arrivals, a single serial model worker: request latency includes queueing.
burst_start=time.perf_counter()
with concurrent.futures.ThreadPoolExecutor(max_workers=1) as pool:
    futures=[pool.submit(infer,100+i) for i in range(4)]
    burst=[]
    for f in futures:
        elapsed,result=f.result()
        burst.append({'request_elapsed_ms':(time.perf_counter()-burst_start)*1000,'inference_ms':elapsed})
measure['burst_4_serial_worker']=burst
measure['ram_final_mb']=proc.memory_info().rss/2**20
stop.set()
measure['ram_peak_sampled_mb']=max(memory,default=0)
if device=='cuda':
    measure['vram_peak_allocated_mb']=torch.cuda.max_memory_allocated()/2**20
    measure['vram_peak_reserved_mb']=torch.cuda.max_memory_reserved()/2**20
    measure['gpu']=torch.cuda.get_device_name()
measure['sample_output']=cold
measure['latencies_ms']=times
dest=Path('local-evidence');dest.mkdir(exist_ok=True)
(dest/f'laya-{device}.json').write_text(json.dumps(measure,indent=2),encoding='utf-8')
print(json.dumps({k:v for k,v in measure.items() if k not in ['latencies_ms','sample_output']},indent=2),flush=True)
