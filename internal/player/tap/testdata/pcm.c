#include <assert.h>
#include <stdlib.h>
#include "pcm.h"
int main(void) {
    utuimp_pcm_tap *t = (utuimp_pcm_tap *)calloc(1, sizeof(*t));
    utuimp_pcm_init(t);
    float *stereo = (float *)calloc(96000 * 2, sizeof(float));
    for (int i=0; i<96000; ++i) { stereo[i*2] = i / 96000.0f; stereo[i*2+1] = stereo[i*2]; }
    utuimp_pcm_write(t, stereo, 96000, 2, 48000, 1);
    float dst[8192]; double ahead; int64_t end;
    assert(utuimp_pcm_read(t, 24000, dst, 8192, &ahead, &end) == 48000);
    assert(fabs(ahead - .18) < 1e-8 && end == 32640);
    assert(fabs(dst[8191] - 32639/96000.0f) < 1e-6);
    assert(utuimp_pcm_read(t, 500000, dst, 8192, &ahead, &end) == 0);
    utuimp_pcm_reset(t, 10000);
    assert(utuimp_pcm_read(t, 10000, dst, 8192, &ahead, &end) == 0);
    int16_t s16[20000]; for (int i=0;i<20000;i++) s16[i]=16384;
    utuimp_pcm_write(t,s16,10000,2,44100,0);
    assert(utuimp_pcm_read(t,12000,dst,8192,&ahead,&end)==44100);
    assert(dst[0]==.5f && dst[8191]==.5f);
    for(int i=0;i<30;i++) utuimp_pcm_write(t,s16,10000,2,44100,0);
    assert(utuimp_pcm_read(t,12000,dst,8192,&ahead,&end)==0);
    assert(utuimp_pcm_read(t,300000,dst,8192,&ahead,&end)==44100);
    utuimp_pcm_destroy(t); free(t); free(stereo);
    return 0;
}
