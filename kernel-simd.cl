__constant static const uint8 uint2x4_mask = (uint8)(0, 4, 1, 5, 2, 6, 3, 7);

inline static uint8 uint2x4f(const uint4 a, const uint4 b){
    return shuffle2(a, b, uint2x4_mask);
}

inline static uint8 ror64(const uint8 x, const uint4 y)
{
    return uint2x4f((x.even>>y)^(x.odd<<(32-y)), (x.odd>>y)^(x.even<<(32-y)));
}

inline static uint8 ror64_2(const uint8 x, const uint4 y)
{
    return uint2x4f((x.odd>>(y-32))^(x.even<<(64-y)), (x.even>>(y-32))^(x.odd<<(64-y)));
}

__constant static const uchar blake2b_sigma[12][16] = {
	{ 0,  1,  2,  3,  4,  5,  6,  7,  8,  9,  10, 11, 12, 13, 14, 15 } ,
	{ 14, 10, 4,  8,  9,  15, 13, 6,  1,  12, 0,  2,  11, 7,  5,  3  } ,
	{ 11, 8,  12, 0,  5,  2,  15, 13, 10, 14, 3,  6,  7,  1,  9,  4  } ,
	{ 7,  9,  3,  1,  13, 12, 11, 14, 2,  6,  5,  10, 4,  0,  15, 8  } ,
	{ 9,  0,  5,  7,  2,  4,  10, 15, 14, 1,  11, 12, 6,  8,  3,  13 } ,
	{ 2,  12, 6,  10, 0,  11, 8,  3,  4,  13, 7,  5,  15, 14, 1,  9  } ,
	{ 12, 5,  1,  15, 14, 13, 4,  10, 0,  7,  6,  3,  9,  2,  8,  11 } ,
	{ 13, 11, 7,  14, 12, 1,  3,  9,  5,  0,  15, 4,  8,  6,  2,  10 } ,
	{ 6,  15, 14, 9,  11, 3,  0,  8,  12, 2,  13, 7,  1,  4,  10, 5  } ,
	{ 10, 2,  8,  4,  7,  6,  1,  5,  15, 11, 9,  14, 3,  12, 13, 0  } ,
	{ 0,  1,  2,  3,  4,  5,  6,  7,  8,  9,  10, 11, 12, 13, 14, 15 } ,
	{ 14, 10, 4,  8,  9,  15, 13, 6,  1,  12, 0,  2,  11, 7,  5,  3  } };
// Target is passed in via headerIn[32 - 29]

__constant static const ulong4 z0 = (ulong4)(0);

__kernel void nonceGrind(__global ulong *headerIn, __global ulong *nonceOut) {
    ulong target = headerIn[10];
    ulong4 num = (ulong4)(get_global_id(0));
    ulong4 off = (ulong4)(get_global_offset(0));
	ulong4 m[16] = {(ulong4)(headerIn[0]), (ulong4)(headerIn[1]),
	                (ulong4)(headerIn[2]), (ulong4)(headerIn[3]),
	                ((num - off) * 4 + (ulong4)(0, 1, 2, 3) + off) | (ulong4)(headerIn[4]), 
                    (ulong4)(headerIn[5]),
	                (ulong4)(headerIn[6]), (ulong4)(headerIn[7]),
	                (ulong4)(headerIn[8]), (ulong4)(headerIn[9]), 
                    z0, z0, z0, z0, z0, z0 
                    };
	ulong4 v[16] = {(ulong4)(0x6a09e667f2bdc928), (ulong4)(0xbb67ae8584caa73b), (ulong4)(0x3c6ef372fe94f82b), (ulong4)(0xa54ff53a5f1d36f1),
	                (ulong4)(0x510e527fade682d1), (ulong4)(0x9b05688c2b3e6c1f), (ulong4)(0x1f83d9abfb41bd6b), (ulong4)(0x5be0cd19137e2179),
	                (ulong4)(0x6a09e667f3bcc908), (ulong4)(0xbb67ae8584caa73b), (ulong4)(0x3c6ef372fe94f82b), (ulong4)(0xa54ff53a5f1d36f1),
	                (ulong4)(0x510e527fade68281), (ulong4)(0x9b05688c2b3e6c1f), (ulong4)(0xe07c265404be4294), (ulong4)(0x5be0cd19137e2179) };
#define G(r,i,a,b,c,d) \
	a = a + b + m[ blake2b_sigma[r][2*i] ]; \
	((uint8*)&d)[0] = ((uint8*)&d)[0].s10325476 ^ ((uint8*)&a)[0].s10325476; \
	c = c + d; \
	((uint8*)&b)[0] = ror64( ((uint8*)&b)[0] ^ ((uint8*)&c)[0], (uint4)(24U)); \
	a = a + b + m[ blake2b_sigma[r][2*i+1] ]; \
	((uint8*)&d)[0] = ror64( ((uint8*)&d)[0] ^ ((uint8*)&a)[0], (uint4)(16U)); \
	c = c + d; \
    ((uint8*)&b)[0] = ror64_2( ((uint8*)&b)[0] ^ ((uint8*)&c)[0], (uint4)(63U));
#define ROUND(r)                    \
	G(r,0,v[ 0],v[ 4],v[ 8],v[12]); \
	G(r,1,v[ 1],v[ 5],v[ 9],v[13]); \
	G(r,2,v[ 2],v[ 6],v[10],v[14]); \
	G(r,3,v[ 3],v[ 7],v[11],v[15]); \
	G(r,4,v[ 0],v[ 5],v[10],v[15]); \
	G(r,5,v[ 1],v[ 6],v[11],v[12]); \
	G(r,6,v[ 2],v[ 7],v[ 8],v[13]); \
	G(r,7,v[ 3],v[ 4],v[ 9],v[14]);
	ROUND( 0 );
	ROUND( 1 );
	ROUND( 2 );
	ROUND( 3 );
	ROUND( 4 );
	ROUND( 5 );
	ROUND( 6 );
	ROUND( 7 );
	ROUND( 8 );
	ROUND( 9 );
	ROUND( 10 );
	ROUND( 11 );
#undef G
#undef ROUND
    barrier(CLK_LOCAL_MEM_FENCE|CLK_GLOBAL_MEM_FENCE);

    ulong4 l = (ulong4)(0x6a09e667f2bdc928) ^ v[0] ^ v[8];
    ulong2 lw = as_ulong2(as_uchar16(l.xy).s76543210fedcba98);
    ulong2 hg = as_ulong2(as_uchar16(l.zw).s76543210fedcba98);
    
	if (lw.x < target) {*nonceOut = m[4].x;return;}
    if (lw.y < target) {*nonceOut = m[4].y;return;}
    if (hg.x < target) {*nonceOut = m[4].z;return;}
    if (hg.y < target) {*nonceOut = m[4].w;return;}
}