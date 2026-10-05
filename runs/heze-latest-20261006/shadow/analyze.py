#!/usr/bin/env python3
import collections,json,pathlib,re,statistics,sys
run=pathlib.Path(sys.argv[1]); data=run/'data'; manifest=json.loads((run/'manifest.json').read_text());slots=manifest['slots']
classes={x['node']:x['class'] for x in manifest['placement']}
tok=re.compile(r'(\w+)=("(?:[^"\\]|\\.)*"|\S+)')
ansi=re.compile(r'\x1b\[[0-9;]*m')
def fields(line):return {k:v[1:-1] if v.startswith('"') else v for k,v in tok.findall(ansi.sub('',line))}
def duration_ms(v):
 units={'ns':1e-6,'us':1e-3,'µs':1e-3,'ms':1,'s':1000,'m':60000,'h':3600000}
 return sum(float(n)*units[u] for n,u in re.findall(r'([\d.]+)(ns|us|µs|ms|s|m|h)',v))
def stats(xs):
 xs=sorted(xs)
 return {'count':len(xs),'min':xs[0],'median':statistics.median(xs),'p95':xs[min(len(xs)-1,int(len(xs)*.95))],'max':xs[-1]} if xs else {}
def read(path):
 try:return json.loads(path.read_text())
 except Exception as e:return {'collection_error':str(e)}
result={'manifest':manifest,'nodes':[],'blocks':[],'builds':[],'proposal_failures':[],'errors_after_genesis':[],'warnings_after_genesis':[],'positive_depth_reorgs':[]}
ledger_counts=collections.Counter(); ledger_samples=[]; node_events={}; errors=collections.Counter();warns=collections.Counter()
for i in range(1,manifest['nodes']+1):
 node=f'node{i}'; events=collections.defaultdict(dict);starts={};gf=[];ffg=[];payload_lat=[];block_lat=[]
 root=read(data/node/'final-block-root.json');head=read(data/node/'final-head.json');sync=read(data/node/'final-syncing.json');finality=read(data/node/'finality.json')
 metric={}
 mpath=data/node/'final-metrics.json'
 if mpath.exists():
  for line in mpath.read_text().splitlines():
   if line.startswith(('p2p_pubsub_undeliverable','p2p_pubsub_reject','p2p_peer_count')):
    parts=line.rsplit(' ',1)
    if len(parts)==2:
     try:metric[parts[0]]=float(parts[1])
     except ValueError:pass
 for role,file in [('beacon',data/node/'prysm/logs/beacon-chain.log'),('validator',data/node/'prysm/logs/validator.log')]:
  if not file.exists():errors[(node,role,'MISSING LOG','')]+=1;continue
  for lineno,line in enumerate(file.open(errors='replace'),1):
   d=fields(line);msg=d.get('msg','');when=d.get('time','')
   if when>='2000-01-01 00:05:00':
    if d.get('level') in ['error','fatal','panic']:errors[(node,role,msg,d.get('error',''))]+=1
    if d.get('level')=='warning':warns[(node,role,msg,d.get('error',''))]+=1
   if role!='beacon':continue
   slot=int(d.get('slot',-1))
   if msg=='Building block':starts[slot]=duration_ms(d['sinceSlotStartTime'])
   elif msg=='Finished building block' and slot in starts and 1<=slot<=slots:
    result['builds'].append({'node':node,'class':classes[node],'slot':slot,'build_ms':duration_ms(d['sinceSlotStartTime'])-starts[slot],'finish_ms_into_slot':duration_ms(d['sinceSlotStartTime']),'line':lineno})
   elif re.search(r'Could not (pack|build)|Fail to build|could not build block',msg,re.I):result['proposal_failures'].append({'node':node,'line':lineno,**d})
   if msg in ['Synced new block','Goldfish votes','FFG votes','Payload received','Block received']:
    events[msg][slot]=d
   if msg=='Goldfish votes' and 1<=slot<slots:gf.append(int(d['seats'])/int(d['committeeSeats']))
   if msg=='FFG votes' and 1<=slot<slots:ffg.append(int(d['seats']))
   if msg=='Payload received' and 1<=slot<=slots:payload_lat.append(int(d['arrivedMs']))
   if msg=='Block received' and 1<=slot<=slots:block_lat.append(int(d['arrivedMs']))
   if msg=='Chain reorg occurred' and int(d.get('depth',0))>0:result['positive_depth_reorgs'].append({'node':node,**d})
   if msg in ['FFG aggregate','PTC vote']:
    vs=int(d.get('attSlot',d.get('slot',-1)))
    if vs==slots-1:
     ledger_counts[(msg,d.get('blockRoot',''),d.get('dataRoot',''))]+=1
     if node=='node1':ledger_samples.append({'node':node,'line':lineno,**d})
 node_events[node]=events
 result['nodes'].append({'node':node,'class':classes[node],'head':head,'root':root,'syncing':sync,'finality':finality,'imported_slots':sorted(s for s in events['Synced new block'] if 1<=s<=slots),'payload_slots':sorted(s for s in events['Payload received'] if 1<=s<=slots),'goldfish_seat_fraction':stats(gf),'ffg_seats':stats(ffg),'payload_arrival_ms':stats(payload_lat),'block_arrival_ms':stats(block_lat),'metrics':metric,'metrics_collected':bool(metric)})
for slot in range(1,slots+1):
 block=read(data/'node1'/f'block-{slot}.json');envelope=read(data/'node1'/f'envelope-{slot}.json');el=read(data/'node1'/f'execution-block-{slot}.json')
 try:
  body=block['data']['message']['body'];payload=envelope['data']['message']['payload'];b=el['result']; atts=body['attestations'];union=0
  for a in atts:
   if int(a['data']['slot'])==slot-1:
    assert a['committee_bits']=='0x0100000000000000'
    bits=int.from_bytes(bytes.fromhex(a['aggregation_bits'][2:]),'little');union|=bits^(1<<(bits.bit_length()-1))
  row={'slot':slot,'proposer':block['data']['message']['proposer_index'],'attestations':len(atts),'payload_attestations':len(body['payload_attestations']),'fresh_participants':union.bit_count(),'fresh_committee_size':manifest['validators']//8,'transactions':len(b['transactions']),'blob_gas_used':int(b.get('blobGasUsed','0x0'),16),'el_hash_matches_envelope':b['hash']==payload['block_hash'],'el_tx_count_matches_envelope':len(b['transactions'])==len(payload['transactions']),'execution_hash':b['hash']}
  result['blocks'].append(row)
 except Exception as e:result['blocks'].append({'slot':slot,'error':str(e)})
for (node,role,msg,error),count in errors.most_common():result['errors_after_genesis'].append({'node':node,'role':role,'message':msg,'error':error,'count':count})
for (node,role,msg,error),count in warns.most_common():result['warnings_after_genesis'].append({'node':node,'role':role,'message':msg,'error':error,'count':count})
roots=[n['root'].get('data',{}).get('root') for n in result['nodes']]
result['all_nodes_agree_fixed_root']=None not in roots and len(set(roots))==1
result['all_nodes_heads_at_requested_slot']=all(n['head'].get('data',{}).get('header',{}).get('message',{}).get('slot')==str(slots) for n in result['nodes'])
result['all_nodes_synced_nonoptimistic']=all(n['syncing'].get('data',{}).get('is_syncing') is False and n['syncing']['data'].get('is_optimistic') is False and n['syncing']['data'].get('sync_distance')=='0' for n in result['nodes'])
validators=read(data/'node1'/'active-validators.json');result['active_validators']=len(validators.get('data',[]))
result['nodes_with_metrics']=sum(n['metrics_collected'] for n in result['nodes'])
result['build_ms']=stats(x['build_ms'] for x in result['builds']);result['ledger_samples_for_last_included_slot']=ledger_samples[:12]
result['ledger_counts_for_last_included_slot']=[{'message':msg,'root':root,'data_root':droot,'lines':count} for (msg,root,droot),count in ledger_counts.items()]
result['by_class']={}
for cls in ['home','super']:
 ns=[n for n in result['nodes'] if n['class']==cls]
 result['by_class'][cls]={'nodes':len(ns),'missing_block_import_node_slots':sum(slots-len(n['imported_slots']) for n in ns),'missing_payload_node_slots':sum(slots-len(n['payload_slots']) for n in ns),'goldfish_fraction_min':min((n['goldfish_seat_fraction'].get('min',0) for n in ns),default=0),'payload_arrival_p95_max_ms':max((n['payload_arrival_ms'].get('p95',0) for n in ns),default=0),'undeliverable_total':sum(v for n in ns for k,v in n['metrics'].items() if k.startswith('p2p_pubsub_undeliverable')),'rejected_total':sum(v for n in ns for k,v in n['metrics'].items() if k.startswith('p2p_pubsub_reject'))}
result['all_requested_blocks_verified']=len(result['blocks'])==slots and all('error' not in b and b['attestations']>0 and b['payload_attestations']>0 and b['el_hash_matches_envelope'] and b['el_tx_count_matches_envelope'] for b in result['blocks'])
(run/'analysis.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps({k:v for k,v in result.items() if k not in ['nodes','manifest','warnings_after_genesis','ledger_samples_for_last_included_slot']},indent=2))
