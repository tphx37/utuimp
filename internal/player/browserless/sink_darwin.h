#ifndef UTUIMP_SINK_DARWIN_H
#define UTUIMP_SINK_DARWIN_H

#ifdef __cplusplus
extern "C" {
#endif

#include <stdint.h>

typedef void* utuimp_darwinsink_t;

typedef void (*utuimp_darwinsink_eos_cb_t)(uintptr_t user_data);
typedef void (*utuimp_darwinsink_err_cb_t)(uintptr_t user_data, char* err_msg);
typedef void (*utuimp_darwinsink_stream_start_cb_t)(uintptr_t user_data, char* uri);

utuimp_darwinsink_t utuimp_darwinsink_create(uintptr_t user_data, utuimp_darwinsink_eos_cb_t on_eos, utuimp_darwinsink_err_cb_t on_err, utuimp_darwinsink_stream_start_cb_t on_stream_start);
void utuimp_darwinsink_play_uri(utuimp_darwinsink_t handle, const char* uri);
void utuimp_darwinsink_set_next_uri(utuimp_darwinsink_t handle, const char* uri);
void utuimp_darwinsink_play(utuimp_darwinsink_t handle);
void utuimp_darwinsink_pause(utuimp_darwinsink_t handle);
void utuimp_darwinsink_stop(utuimp_darwinsink_t handle);
void utuimp_darwinsink_seek(utuimp_darwinsink_t handle, double seconds);
void utuimp_darwinsink_set_volume(utuimp_darwinsink_t handle, float volume);
void utuimp_darwinsink_set_eq(utuimp_darwinsink_t handle, const double* gains_db, int count);
double utuimp_darwinsink_position(utuimp_darwinsink_t handle);
int utuimp_darwinsink_read_tap(utuimp_darwinsink_t handle, float* dst, int n, double* ahead, int64_t* end);
void utuimp_darwinsink_destroy(utuimp_darwinsink_t handle);

#ifdef __cplusplus
}
#endif

#endif // UTUIMP_SINK_DARWIN_H
