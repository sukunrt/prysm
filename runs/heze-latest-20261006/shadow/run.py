#!/usr/bin/env python3
import importlib.util,json,pathlib,random,shlex,shutil,subprocess,sys
import yaml
root=pathlib.Path(__file__).resolve().parents[3]
slots=int(sys.argv[1]) if len(sys.argv)>1 else 32
attempt=int(sys.argv[2]) if len(sys.argv)>2 else 1
name=f'heze-n10-v1000-super10-s1-{slots}slots'+(f'-r{attempt}' if attempt>1 else '')
spec=importlib.util.spec_from_file_location('packing_harness',root/'shadow/run-shadow-sim.py')
h=importlib.util.module_from_spec(spec);spec.loader.exec_module(h)
old_place=h.place
placement=[]
def place(args,rng):
 country,supers,_=old_place(args,rng)
 vals=[100]*args.nodes
 assert sum(vals)==args.validators and len(supers)==10
 placement.extend({'node':f'node{i+1}','country':country[i],'class':'super' if i in supers else 'home','validators':vals[i]} for i in range(args.nodes))
 return country,supers,vals
h.place=place
old_config=h.sim_config
def config(*args):
 cfg=old_config(*args)
 cfg['ethereum']['genesis']['generator_image']='prysm-genesis-gen:heze-latest-20261006'
 cfg['ethereum']['clients']['prometheus']={'type':'prometheus','executable':shutil.which('prometheus')}
 return cfg
h.sim_config=config
sys.argv=[str(root/'shadow/run-shadow-sim.py'),'--nodes','10','--validators','1000','--supernode-fraction','1','--duration',str(slots*12+20),'--seed','1','--name',name,'--gen-only']
h.main()
out=root/'shadow/runs'/name;data=out/'data'
sim=yaml.safe_load((out/'sim.yaml').read_text());chain=yaml.safe_load((data/'metadata/config.yaml').read_text())
nodes=[n for n in sim['ethereum']['nodes'] if 'vc' in n['clients']]
assert len(nodes)==10 and sim['ethereum']['validators']==1000
assert sum(n['clients']['cl']=='prysm_super' for n in nodes)==10
assert sum(n['reliability']=='staker' for n in nodes)==0
assert all(sim['ethereum']['clients'][n['clients']['vc']]['validators']==100 for n in nodes)
assert chain['MIN_GENESIS_ACTIVE_VALIDATOR_COUNT']==1000 and chain['HEZE_FORK_EPOCH']==0
shadow_path=data/'shadow.yaml';shadow=yaml.safe_load(shadow_path.read_text())
for row in placement:
 procs=shadow['hosts'][row['node']]['processes']
 beacon=next(p for p in procs if p['path'].endswith('prysm-beacon'))
 assert ('--supernode' in beacon['args'])==(row['class']=='super')
def capture(node,label,start,url,post=None):
 args=['--silent','--show-error','--fail','--max-time','5','--output',str(data/node/(label+'.json'))]
 if post is not None:args+=['-H','Content-Type: application/json','--data',json.dumps(post)]
 args+=[url]
 shadow['hosts'][node]['processes'].append({'path':'/usr/bin/curl','args':shlex.join(args),'environment':{},'start_time':f'{start}s','expected_final_state':{'exited':0}})
final=300+slots*12
for i in range(1,11):
 node=f'node{i}'
 capture(node,'final-head',final+10,'http://127.0.0.1:31001/eth/v1/beacon/headers/head')
 capture(node,'final-block-root',final+10,f'http://127.0.0.1:31001/eth/v1/beacon/blocks/{slots}/root')
 capture(node,'final-syncing',final+10,'http://127.0.0.1:31001/eth/v1/node/syncing')
 capture(node,'finality',final+10,'http://127.0.0.1:31001/eth/v1/beacon/states/head/finality_checkpoints')
 beacon=next(p for p in shadow['hosts'][node]['processes'] if p['path'].endswith('prysm-beacon'))
 args=shlex.split(beacon['args'])
 monitoring_host=args[args.index('--monitoring-host')+1]
 monitoring_port=args[args.index('--monitoring-port')+1]
 capture(node,'final-metrics',final+9,f'http://{monitoring_host}:{monitoring_port}/metrics')
capture('node1','active-validators',final+8,'http://127.0.0.1:31001/eth/v1/beacon/states/head/validators?status=active')
capture('node1','runtime-spec',final+8,'http://127.0.0.1:31001/eth/v1/config/spec')
for slot in range(1,slots+1):
 t=300+slot*12+10
 capture('node1',f'block-{slot}',t,f'http://127.0.0.1:31001/eth/v2/beacon/blocks/{slot}')
 capture('node1',f'envelope-{slot}',t,f'http://127.0.0.1:31001/eth/v1/beacon/execution_payload_envelopes/{slot}')
 capture('node1',f'execution-block-{slot}',t,'http://127.0.0.1:22001',{'jsonrpc':'2.0','id':slot,'method':'eth_getBlockByNumber','params':[hex(slot),False]})
shadow_path.write_text(yaml.safe_dump(shadow,sort_keys=False))
manifest={'run_name':name,'source_root':str(root),'nodes':10,'validators':1000,'validators_per_node':100,'home_nodes':0,'supernodes':10,'slots':slots,'seed':1,'placement':placement,'home_bandwidth':'25 Mbit up / 50 Mbit down','super_bandwidth':'1024 Mbit up/down','duration_after_genesis_seconds':slots*12+20,'chain_config':{k:chain.get(k) for k in ['HEZE_FORK_EPOCH','SLOTS_PER_EPOCH','SLOTS_PER_ROUND','TARGET_COMMITTEE_SIZE','TARGET_AGGREGATORS_PER_COMMITTEE','ATTESTATION_SUBNET_COUNT','SUBNETS_PER_NODE','AGGREGATE_DUE_BPS_GLOAS']},'cpu_model':'Shadow models network, not real CPU cost','observations':'final heads/roots/sync/finality/metrics on all nodes; each requested block/envelope and matching EL block number on node1'}
(out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
print('PREFLIGHT_OK',json.dumps({k:v for k,v in manifest.items() if k!='placement'}),flush=True)
h.run(['shadow','-d',str(data/'shadow'),str(shadow_path)],log=out/'shadow.log')
print('SHADOW_COMPLETE',name,flush=True)
