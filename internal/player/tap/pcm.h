#ifndef UTUIMP_PCM_TAP_H
#define UTUIMP_PCM_TAP_H

// Native outputs keep decoded mono PCM until the device plays it. Reading
// follows the device clock, with the same 180 ms lookahead as the Linux tap.
// The audio thread never calls Go or allocates memory here.
#include <stdint.h>
#include <stddef.h>
#include <math.h>
#ifdef _WIN32
#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN
#endif
#include <windows.h>
#include <mmsystem.h>
typedef SRWLOCK utuimp_pcm_mutex;
#define UTUIMP_PCM_INIT(m) InitializeSRWLock(m)
#define UTUIMP_PCM_LOCK(m) AcquireSRWLockExclusive(m)
#define UTUIMP_PCM_UNLOCK(m) ReleaseSRWLockExclusive(m)
#define UTUIMP_PCM_DESTROY(m) ((void)(m))
#else
#include <pthread.h>
typedef pthread_mutex_t utuimp_pcm_mutex;
#define UTUIMP_PCM_INIT(m) pthread_mutex_init(m, NULL)
#define UTUIMP_PCM_LOCK(m) pthread_mutex_lock(m)
#define UTUIMP_PCM_UNLOCK(m) pthread_mutex_unlock(m)
#define UTUIMP_PCM_DESTROY(m) pthread_mutex_destroy(m)
#endif

#ifdef _WIN32
static inline double utuimp_pcm_wave_position(HWAVEOUT wave, uint32_t rate, uint32_t bytes_per_sec) {
    MMTIME t = {0}; t.wType = TIME_SAMPLES;
    if (waveOutGetPosition(wave, &t, sizeof(t)) != MMSYSERR_NOERROR) return -1;
    if (t.wType == TIME_SAMPLES) return t.u.sample;
    if (t.wType == TIME_BYTES && bytes_per_sec > 0) return (double)t.u.cb * rate / bytes_per_sec;
    if (t.wType == TIME_MS) return (double)t.u.ms * rate / 1000;
    return -1;
}
#endif

#define UTUIMP_PCM_CAPACITY (1u << 18)
typedef struct {
    utuimp_pcm_mutex mu;
    float samples[UTUIMP_PCM_CAPACITY];
    int rate;
    uint64_t written;
    double origin;
} utuimp_pcm_tap;

static inline void utuimp_pcm_init(utuimp_pcm_tap *t) {
    UTUIMP_PCM_INIT(&t->mu);
    t->rate = 0; t->written = 0; t->origin = 0;
}
static inline void utuimp_pcm_destroy(utuimp_pcm_tap *t) { UTUIMP_PCM_DESTROY(&t->mu); }
static inline void utuimp_pcm_reset(utuimp_pcm_tap *t, double origin) {
    UTUIMP_PCM_LOCK(&t->mu);
    t->rate = 0; t->written = 0; t->origin = origin;
    UTUIMP_PCM_UNLOCK(&t->mu);
}

// f32 is interleaved native float; s16 is interleaved signed 16-bit PCM.
static inline void utuimp_pcm_write(utuimp_pcm_tap *t, const void *pcm, size_t frames, int channels, int rate, int f32) {
    if (!pcm || channels <= 0 || rate <= 0) return;
    UTUIMP_PCM_LOCK(&t->mu);
    if (t->rate != 0 && t->rate != rate) t->written = 0;
    t->rate = rate;
    for (size_t f = 0; f < frames; ++f) {
        double mono = 0;
        for (int c = 0; c < channels; ++c) {
            size_t i = f * (size_t)channels + (size_t)c;
            mono += f32 ? ((const float *)pcm)[i] : ((const int16_t *)pcm)[i] / 32768.0;
        }
        mono /= channels;
        if (!isfinite(mono)) mono = 0;
        if (mono > 1) mono = 1;
        if (mono < -1) mono = -1;
        t->samples[t->written++ % UTUIMP_PCM_CAPACITY] = (float)mono;
    }
    UTUIMP_PCM_UNLOCK(&t->mu);
}

// played is the device's absolute frame position. Returning zero for stale
// or overwritten windows prevents playing old beats after a seek/underrun.
static inline int utuimp_pcm_read(utuimp_pcm_tap *t, double played, float *dst, int n, double *ahead, int64_t *end) {
    if (!dst || n <= 0 || n > (int)UTUIMP_PCM_CAPACITY / 2 || !isfinite(played)) return 0;
    UTUIMP_PCM_LOCK(&t->mu);
    int rate = t->rate;
    double heard = played - t->origin;
    if (rate <= 0 || heard < 0 || heard > (double)t->written) {
        UTUIMP_PCM_UNLOCK(&t->mu); return 0;
    }
    uint64_t stop = (uint64_t)(heard + 0.18 * rate);
    if (stop > t->written) stop = t->written;
    if (stop < (uint64_t)n || t->written - (stop - (uint64_t)n) > UTUIMP_PCM_CAPACITY) {
        UTUIMP_PCM_UNLOCK(&t->mu); return 0;
    }
    for (int i = 0; i < n; ++i) dst[i] = t->samples[(stop - (uint64_t)n + (uint64_t)i) % UTUIMP_PCM_CAPACITY];
    *ahead = ((double)stop - heard) / rate;
    *end = (int64_t)stop;
    UTUIMP_PCM_UNLOCK(&t->mu);
    return rate;
}
#endif
