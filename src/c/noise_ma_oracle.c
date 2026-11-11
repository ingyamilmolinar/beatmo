/* Test-support oracle: fills a buffer with the REAL ma_noise white stream
 * so noise_ma_parity_test.go can prove the noise.c transplant bit-identical.
 * Not referenced by any production code path.
 *
 * Header-only include (mirrors drums.c): MINIAUDIO_IMPLEMENTATION lives in
 * miniaudio.c, so this shim links against build/miniaudio.o. */
#define MA_NO_DEVICE_IO
#define MA_NO_THREADING
#include "miniaudio.h"

void oracle_ma_noise_white_fill(float *out, int n, int seed) {
    ma_noise_config nc = ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, seed, 1.0);
    ma_noise noise;
    if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) return;
    for (int i = 0; i < n; i++) {
        ma_noise_read_pcm_frames(&noise, &out[i], 1, NULL);
    }
    ma_noise_uninit(&noise, NULL);
}
