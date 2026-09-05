# Round 2 slots 0–100: production and retention timeline

This table separates a proposer’s local construction stages from the earliest block import in the complete 1,000-node archive union and from later positive-depth reorg logs. Times are offsets from each slot start. Slot 0 is genesis, not a proposal failure.

A row with an import proves that a block was observed; it does not alone establish long-term canonical retention. “Reorg-out” means at least one captured node later logged a positive-depth transition away from a head at that slot by wall slot 101. It is deliberately not called a network-canonical result. The parser is [round2_slots_0_100.py](/home/sukun/dev/prysm2/runs/diagnostics/startup3/round2_slots_0_100.py).

## Stage breakdown for the bypassed slow blocks

For the 13 later blocks that were both slower than the ordinary fast path and
subsequently bypassed in the independently reconstructed parent lineage, the
build RPC began promptly and selected a payload source promptly.  The dominant
logged interval was consistently **payload chosen to `Finished building
block`**, not pre-build dispatch or post-finish publication:

| slot | build start | choose - build | finish - choose | import - finish | submit - import |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 34 | +0.022s | 0.003s | 3.965s | 0.250s | 0.126s |
| 39 | +0.011s | 0.006s | 2.897s | 0.255s | 0.068s |
| 45 | +0.015s | 0.004s | 3.302s | 0.219s | 0.057s |
| 65 | +0.019s | 0.002s | 2.749s | 0.917s | 0.257s |
| 68 | +0.014s | 0.006s | 7.763s | 0.136s | 0.204s |
| 70 | +0.020s | 0.011s | 4.389s | 0.223s | 0.421s |
| 74 | +0.015s | 0.009s | 6.033s | 0.118s | 0.238s |
| 75 | +0.009s | 0.031s | 4.412s | 0.119s | 0.150s |
| 78 | +0.013s | 0.006s | 4.440s | 0.194s | 0.032s |
| 81 | +0.017s | 0.017s | 3.891s | 0.093s | 0.312s |
| 82 | +0.011s | 0.021s | 2.542s | 0.140s | 0.094s |
| 97 | +0.009s | 0.008s | 3.646s | 0.226s | 0.288s |
| 100 | +0.019s | 0.011s | 3.923s | 0.116s | 0.315s |

That interval is a joined post-selection construction region: the existing
logs do not isolate payload packing CPU from state-root calculation, state
transition work, scheduling delay, or locks inside the region.  It must not be
reported as payload-packing CPU alone.  Slot 65 also has the largest observed
finish-to-first-import interval (0.917s), but its 2.749s joined construction
interval remains the largest stage.

Slot 82 is an important discriminator.  Owner 200 finished by +2.574s, the
union first imported the block by +2.714s, and the VC submitted by +2.808s.
The owner then logged prompt local import and envelope publication/sync.  Its
later bypass therefore is not explained by a missed build deadline or an
injected publication wait; it belongs to subsequent availability/fork-choice
retention.  Conversely, the table alone does not prove that the longer joined
construction intervals caused the other lineage bypasses.

| slot | duty owner(s) | BN build | payload chosen | BN finish | earliest import | VC submit | injected late publish | observed reorg-out |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| 0 | -- | -- | -- | -- | -- | -- | -- | -- |
| 1 | 169 | -- | -- | -- | -- | -- | -- | -- |
| 2 | 191 | -- | -- | -- | -- | -- | -- | -- |
| 3 | 22 | -- | -- | -- | -- | -- | -- | -- |
| 4 | 91 | -- | -- | -- | -- | -- | -- | -- |
| 5 | 118 | +10.120s | -- | -- | -- | -- | -- | -- |
| 6 | 83 | +3.664s | -- | -- | -- | -- | -- | -- |
| 7 | 144 | -- | -- | -- | -- | -- | -- | -- |
| 8 | 19 | +11.191s | -- | -- | -- | -- | -- | -- |
| 9 | 107 | +5.983s | -- | -- | -- | -- | -- | -- |
| 10 | 35 | +7.447s | +10.338s | -- | -- | -- | -- | -- |
| 11 | 117 | -- | -- | -- | -- | -- | -- | -- |
| 12 | 14 | -- | -- | -- | -- | -- | -- | -- |
| 13 | 20 | +1.317s | -- | -- | -- | -- | -- | -- |
| 14 | 85 | +2.694s | +2.744s | -- | -- | -- | -- | -- |
| 15 | 32 | +0.009s | +0.019s | +1.482s | +1.592s | +1.659s | -- | wall 16 (940 logs; to 0,16) |
| 16 | 76 | +0.219s | +0.704s | +1.817s | +1.924s | +6.774s | -- | wall 17 (937 logs; to 15,17) |
| 17 | 99 | +0.057s | +0.596s | +2.446s | +2.567s | +2.745s | -- | wall 17 (1019 logs; to 15,16,18,20) |
| 18 | 49 | +3.004s | +4.974s | +9.887s | +10.011s | +10.927s | -- | wall 19 (930 logs; to 15,20,21,22) |
| 19 | 142 | +4.167s | +5.780s | +11.946s | +12.346s | -- | -- | -- |
| 20 | 141 | +3.037s | +3.043s | +5.290s | +5.599s | +6.323s | -- | wall 21 (969 logs; to 15,18) |
| 21 | 35 | +0.011s | +0.042s | +0.391s | +0.656s | +0.788s | -- | wall 22 (3 logs; to 18) |
| 22 | 181 | +0.014s | +0.024s | +0.589s | +0.779s | +0.851s | -- | -- |
| 23 | 25 | +0.005s | +0.017s | +0.467s | +0.733s | +0.813s | -- | -- |
| 24 | 138 | +0.012s | +0.030s | +0.419s | +0.565s | +0.660s | -- | -- |
| 25 | 96 | +0.009s | +0.013s | +0.186s | +0.301s | +0.435s | -- | -- |
| 26 | 175 | +0.016s | +0.028s | +0.641s | +0.884s | +1.296s | -- | -- |
| 27 | 198 | +0.011s | +0.017s | +0.211s | +0.310s | +0.319s | -- | -- |
| 28 | 76 | +0.008s | +0.012s | +0.208s | +0.328s | +0.444s | -- | -- |
| 29 | 103 | +0.006s | +0.011s | +0.267s | +0.393s | +0.568s | -- | -- |
| 30 | 157 | +0.012s | +0.020s | +0.469s | +0.691s | +1.061s | -- | -- |
| 31 | 93 | +0.014s | +0.018s | +0.347s | +0.548s | +0.601s | -- | -- |
| 32 | 11 | +0.277s | +0.301s | +1.281s | +1.556s | +1.827s | -- | -- |
| 33 | 70 | +0.007s | +0.018s | +0.618s | +1.075s | +3.792s | -- | -- |
| 34 | 44 | +0.022s | +0.025s | +3.990s | +4.240s | +4.366s | -- | wall 35 (1000 logs; to 33) |
| 35 | 23 | +0.011s | +0.044s | +1.155s | +1.500s | +2.113s | -- | -- |
| 36 | 16 | +0.005s | +0.014s | +0.358s | +0.669s | +0.920s | -- | -- |
| 37 | 18 | +0.014s | +0.016s | +1.764s | +2.018s | +2.161s | -- | -- |
| 38 | 87 | +0.010s | +0.018s | +0.287s | +0.559s | +0.756s | -- | -- |
| 39 | 83 | +0.011s | +0.017s | +2.914s | +3.169s | +3.237s | -- | wall 40 (1000 logs; to 38) |
| 40 | 129 | +0.010s | +0.061s | +0.611s | +0.967s | +1.148s | -- | -- |
| 41 | 171 | +0.010s | +0.016s | +0.338s | +0.637s | +1.049s | -- | -- |
| 42 | 85 | +0.013s | +0.019s | +0.952s | +1.199s | +1.647s | -- | -- |
| 43 | 93 | +0.011s | +0.017s | +0.339s | +0.670s | +0.842s | -- | -- |
| 44 | 30 | +0.007s | +0.014s | +0.513s | +0.856s | +2.044s | -- | -- |
| 45 | 169 | +0.015s | +0.019s | +3.321s | +3.540s | +3.597s | -- | wall 46 (1000 logs; to 44) |
| 46 | 85 | +0.015s | +0.035s | +1.466s | +1.726s | +2.033s | -- | -- |
| 47 | 120 | +0.009s | +0.016s | +0.274s | +0.523s | +0.638s | -- | -- |
| 48 | 46 | +0.010s | +0.030s | +0.541s | +0.780s | +0.924s | -- | -- |
| 49 | 172 | +0.012s | +0.019s | +0.259s | +0.600s | +0.669s | -- | -- |
| 50 | 155 | +0.015s | +0.024s | +0.667s | +1.171s | +3.728s | -- | wall 51 (1 logs; to 49) |
| 51 | 114 | +0.018s | +0.030s | +0.508s | +0.789s | +1.141s | -- | wall 52 (999 logs; to 50) |
| 52 | 161 | +0.009s | +0.034s | +0.347s | +0.598s | +0.822s | -- | -- |
| 53 | 189 | +0.006s | +0.018s | +0.727s | +1.348s | +1.723s | -- | -- |
| 54 | 16 | +0.004s | +0.017s | +0.419s | +0.702s | +1.335s | -- | -- |
| 55 | 44 | +0.015s | +0.021s | +0.543s | +0.898s | +2.680s | -- | -- |
| 56 | 161 | +0.007s | +0.021s | +0.689s | +0.950s | +1.312s | -- | -- |
| 57 | 154 | +0.016s | +0.023s | +0.414s | +0.715s | +1.953s | -- | -- |
| 58 | 72 | +0.010s | +0.016s | +0.282s | +0.601s | +0.769s | -- | -- |
| 59 | 127 | +0.021s | +0.027s | +1.809s | +2.061s | +2.577s | -- | -- |
| 60 | 77 | +0.006s | +0.019s | +0.401s | +0.728s | +1.809s | -- | -- |
| 61 | 42 | +0.018s | +0.025s | +0.315s | +0.625s | +0.698s | -- | -- |
| 62 | 171 | +0.011s | +0.015s | +2.513s | +2.681s | +2.770s | -- | -- |
| 63 | 129 | +0.013s | +0.019s | +0.278s | +0.567s | +0.602s | -- | -- |
| 64 | 121 | +0.216s | +0.222s | +0.978s | +1.313s | +2.518s | -- | -- |
| 65 | 110 | +0.019s | +0.021s | +2.770s | +3.687s | +3.944s | -- | wall 66 (1000 logs; to 64) |
| 66 | 185 | +0.011s | +0.020s | +0.186s | +0.467s | +0.730s | -- | -- |
| 67 | 166 | +0.011s | +0.033s | +0.294s | +0.624s | +0.925s | -- | -- |
| 68 | 160 | +0.014s | +0.020s | +7.783s | +7.919s | +8.123s | -- | wall 69 (1000 logs; to 67) |
| 69 | 130 | +0.007s | +0.030s | +0.430s | +0.842s | +1.223s | -- | -- |
| 70 | 112 | +0.020s | +0.031s | +4.420s | +4.643s | +5.064s | -- | wall 71 (999 logs; to 69) |
| 71 | 39 | +0.012s | +0.045s | +0.594s | +0.889s | +2.556s | -- | -- |
| 72 | 197 | +0.014s | +0.027s | +0.341s | +0.493s | +0.628s | -- | -- |
| 73 | 142 | +0.009s | +0.017s | +0.298s | +0.520s | +0.897s | -- | -- |
| 74 | 8 | +0.015s | +0.024s | +6.057s | +6.175s | +6.413s | -- | wall 75 (999 logs; to 73) |
| 75 | 149 | +0.009s | +0.040s | +4.452s | +4.571s | +4.721s | -- | wall 76 (999 logs; to 73) |
| 76 | 79 | +0.019s | +0.068s | +0.641s | +0.977s | +1.772s | -- | -- |
| 77 | 54 | +0.007s | +0.022s | +0.359s | +0.634s | +0.709s | -- | -- |
| 78 | 14 | +0.013s | +0.019s | +4.459s | +4.653s | +4.685s | -- | wall 79 (999 logs; to 77) |
| 79 | 116 | +0.007s | +0.022s | +0.536s | +0.759s | +1.176s | -- | -- |
| 80 | 24 | +0.014s | +0.035s | +0.679s | +0.911s | +1.149s | -- | -- |
| 81 | 152 | +0.017s | +0.034s | +3.925s | +4.018s | +4.330s | -- | wall 82 (999 logs; to 80) |
| 82 | 200 | +0.011s | +0.032s | +2.574s | +2.714s | +2.808s | -- | wall 83 (999 logs; to 80) |
| 83 | 165 | +0.008s | +0.042s | +0.279s | +0.526s | +0.607s | -- | -- |
| 84 | 147 | +0.012s | +0.019s | +0.240s | +0.463s | +0.485s | -- | -- |
| 85 | 4 | +0.011s | +0.020s | +0.305s | +0.508s | +0.646s | -- | -- |
| 86 | 59 | +0.011s | +0.018s | +0.371s | +0.621s | +0.976s | -- | -- |
| 87 | 11 | +0.011s | +0.020s | +0.457s | +0.730s | +1.777s | -- | -- |
| 88 | 78 | +0.030s | +0.054s | +0.784s | +1.059s | +1.478s | -- | -- |
| 89 | 117 | +0.012s | +0.019s | +0.267s | +0.541s | +0.670s | -- | -- |
| 90 | 102 | +0.014s | +0.021s | +0.393s | +0.656s | +0.904s | -- | -- |
| 91 | 191 | +0.010s | +0.017s | +0.264s | +0.510s | +0.661s | -- | -- |
| 92 | 125 | +0.012s | +0.018s | +0.456s | +0.777s | +1.573s | -- | -- |
| 93 | 193 | +0.009s | +0.015s | +0.559s | +0.813s | +1.167s | -- | -- |
| 94 | 192 | +0.008s | +0.014s | +0.308s | +0.594s | +0.720s | -- | -- |
| 95 | 191 | +0.009s | +0.020s | +0.241s | +0.518s | +0.596s | -- | -- |
| 96 | 148 | +0.146s | +0.158s | +1.015s | +1.279s | +2.401s | -- | -- |
| 97 | 1 | +0.009s | +0.017s | +3.663s | +3.889s | +4.177s | -- | wall 98 (999 logs; to 96) |
| 98 | 54 | +0.008s | +0.039s | +0.343s | +0.737s | +0.749s | -- | -- |
| 99 | 151 | +0.012s | +0.022s | +0.286s | +0.588s | +0.991s | -- | -- |
| 100 | 143 | +0.019s | +0.030s | +3.953s | +4.069s | +4.384s | -- | wall 101 (999 logs; to 99) |


## Reading the table

- Slots 1–14 have no imported block; the detailed terminal stages are analyzed in [round2-state-payload-deep-audit.md](/home/sukun/dev/prysm2/runs/diagnostics/startup3/round2-state-payload-deep-audit.md) and the startup causal report.
- “BN build” is handler entry, “payload chosen” is an explicit owner-BN selection log, and “BN finish” is successful block construction. Missing stage logs are not synthesized.
- Earliest import is a union observation across all nodes. VC submit is the owner RPC’s completion log and may occur after peers imported the block.
- No selected owner VC logs `Publishing block late` for slots 0–100. The measured latencies therefore are not attributed to the optional injected late-publish wait.
- Reorg counts are log observations, not unique roots or votes. Repeated nodes/views can produce multiple events.
- The separate envelope-parent lineage audit establishes that 19 produced slots (16–20, 34, 39, 45, 51, 65, 68, 70, 74–75, 78, 81–82, 97, and 100) were bypassed by later retained ancestry. This table does not infer that solely from reorg logs.
