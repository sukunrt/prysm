# Current mainnet system:
1m validators over 10k beacon nodes. 
2k super nodes hosting most of the validators
8k home stakers hosting 1-2 validators each

committees = 64
subnets = 64
nodes per subnet = 300 (20k/64)
slots to finality = 32
validators per committee = 500
agg size = ~510 B (SignedAggregateAndProofElectra: 63 B bitlist for 500 bits,
  128 B data, 3 x 96 B sigs, 8 B committee bits, 20 B index + offsets)
number of aggregates = 16 * 64 = 1024 per slot
raw attestations per slot = 64 * 500 = 32k (500 per subnet)


# Target mainnet system with decoupled consensus
1m validators over 10k beacon nodes. 
2k super nodes hosting most of the validators
8k home stakers hosting 1-2 validators each

committees = 64
subnets = 64
slots to finality = 8
attesters per committee = 2000 (1m / 8 slots / 64 committees)
aggregators per committee = 64 (TARGET_AGGREGATORS_PER_COMMITTEE, the sim default)
agg size = ~700 B (251 B bitlist for 2000 bits, the rest as above)
number of aggregates = 16 * 64 = 1024 per slot, ~0.5 MB on the global topic
raw attestations per slot = 64 * 2000 = 128k (2000 per subnet)

# Sim system
120k validators (6 committees * 2500 attesters * 8 slots per round)
200 super nodes hosting 596 validators each (120k - 800, split evenly)
800 home stakers hosting 1 validator each

committees = 6 (one per subnet, as mainnet)
subnets = 6 (this ensures each subnet has as many nodes as mainnet ~= 1k nodes * 2 subnets each / 6 ~= 333)
slots to finality = 8
attesters per committee = 2500 (per-slot pool 15k / 6)
aggregators per committee = 64 
agg size = ~760 B (313 B bitlist for 2500 bits, the rest as above)
number of aggregates = 64 * committees = 384 per slot, ~0.3 MB on the global topic
raw attestations per slot = 6 * 2500 = 15k (2500 per subnet)
