import struct, math, wave, os, sys

outdir = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "artifacts", "smoke")
os.makedirs(outdir, exist_ok=True)

sr = 16000
dur = 3.0
n = int(sr * dur)

with wave.open(os.path.join(outdir, "test_speech.wav"), "w") as w:
    w.setnchannels(1)
    w.setsampwidth(2)
    w.setframerate(sr)
    frames = b""
    for i in range(n):
        t = i / sr
        v = int(16000 * (0.5 * math.sin(2*math.pi*440*t) + 0.3 * math.sin(2*math.pi*880*t) + 0.2 * math.sin(2*math.pi*1320*t)))
        frames += struct.pack("<h", max(-32768, min(32767, v)))
    w.writeframes(frames)

size = os.path.getsize(os.path.join(outdir, "test_speech.wav"))
print(f"WAV generated: {size} bytes, {dur}s, {sr}Hz, mono")
