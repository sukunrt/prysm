#!/usr/bin/env python3
import collections,json,pathlib,re,statistics
out=pathlib.Path(__file__).resolve().parent
checks=json.loads((out/'checks.json').read_text())
ansi=re.compile(r'\x1b\[[0-9;]*m')

def field(line,key):
 m=re.search(r'(?:^|\s)'+re.escape(key)+r'=(?:"([^"]*)"|([^\s]+))',line)
 return (m.group(1) if m.group(1) is not None else m.group(2)) if m else None

def duration_ms(value):
 units={'ns':1e-6,'us':1e-3,'µs':1e-3,'μs':1e-3,'ms':1,'s':1000,'m':60000,'h':3600000}
 return sum(float(n)*units[u] for n,u in re.findall(r'([\d.]+)(ns|us|µs|μs|ms|s|m|h)',value))

builds=[]; failures=[]; ledgers=[]; error_counts=collections.Counter()
for p in sorted(out.glob('cl-*.log')):
 starts={}
 for i,raw in enumerate(p.read_text().splitlines(),1):
  line=ansi.sub('',raw)
  if re.search(r'\bERROR\b|level=error',line): error_counts[p.stem]+=1
  slot=field(line,'slot')
  offset=field(line,'sinceSlotStartTime')
  if ' Building block ' in line and offset:
   starts[slot]=duration_ms(offset)
  if ' Finished building block ' in line and offset and slot in starts and 1<=int(slot)<=32:
   builds.append({'node':p.stem,'slot':int(slot),'build_ms':duration_ms(offset)-starts[slot],'finish_ms_into_slot':duration_ms(offset),'log_line':i})
  if re.search(r'Could not (?:pack|build)|Fail to build|could not build block|Could not request (?:block|beacon block)',line,re.I):
   failures.append({'node':p.stem,'line':i,'text':line})
  if ' FFG aggregate ' in line and ' FFG aggregate groups ' not in line or ' PTC vote ' in line:
   ledgers.append((p.name,i,line))

slots=[]
for s in checks['slots']:
 slot=s['slot']
 body=json.loads((out/f'block-{slot}.json').read_text())['data']['message']['body']
 atts=body.get('attestations',[])
 entries=[]
 for a in atts:
  bits=bytes.fromhex(a['aggregation_bits'][2:])
  entries.append({'slot':int(a['data']['slot']),'participants':sum(x.bit_count() for x in bits)-1,'committee_bits':a.get('committee_bits'),'block_root':a['data']['beacon_block_root']})
 pa=body.get('payload_attestations',[])
 s=dict(s,attestation_details=entries,payload_attestation_details=pa)
 slots.append(s)

# Join an observed beacon block to its EL payload and preceding-slot FFG/PTC ledger.
selected=next(s for s in reversed(slots) if s['transactions'] and s['attestations'] and s['payload_attestations'])
ffg=[];ptc=[]
for name,i,line in ledgers:
 if ' FFG aggregate ' in line and ' FFG aggregate groups ' not in line:
  if any(field(line,'attSlot')==str(a['slot']) and field(line,'blockRoot')==a['block_root'] for a in selected['attestation_details'] if a['slot']==selected['slot']-1):
   ffg.append({'file':name,'line':i,'text':line})
 else:
  for pa in selected['payload_attestation_details']:
   d=pa.get('data',{})
   if field(line,'slot')==str(d.get('slot')) and field(line,'blockRoot')==d.get('beacon_block_root'):
    ptc.append({'file':name,'line':i,'text':line})
    break
report={'builds':sorted(builds,key=lambda x:x['slot']),'build_median_ms':statistics.median(x['build_ms'] for x in builds) if builds else None,'build_max_ms':max((x['build_ms'] for x in builds),default=None),'proposal_failures':failures,'beacon_error_line_counts':dict(error_counts),'slots':slots,'joined_slot':selected['slot'],'ffg_matching_log_lines':len(ffg),'ptc_matching_log_lines':len(ptc),'ptc_matching_unique_validators':len({field(x['text'],'validatorIndex') for x in ptc}),'ffg_examples':ffg[:3],'ptc_examples':ptc[:3]}
(out/'audit.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({k:v for k,v in report.items() if k not in ['slots','ffg_examples','ptc_examples']},indent=2))
print('Slot details:',json.dumps([{k:v for k,v in s.items() if k!='payload_attestation_details'} for s in slots],indent=2))
if not ffg or not ptc or failures or len(builds)!=32:
 raise SystemExit(1)
