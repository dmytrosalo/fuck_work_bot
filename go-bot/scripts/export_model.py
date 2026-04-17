"""
Export sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2 to ONNX format.
Keeps only essential files for Go inference.
"""

import os
import shutil
import subprocess
import sys

MODEL_NAME = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
MODEL_DIR = os.path.join(SCRIPT_DIR, "..", "model")
TMP_DIR = os.path.join(SCRIPT_DIR, "..", "model_tmp")

KEEP_FILES = {
    "model.onnx",
    "tokenizer.json",
    "config.json",
    "special_tokens_map.json",
    "tokenizer_config.json",
}


def main():
    os.makedirs(MODEL_DIR, exist_ok=True)
    os.makedirs(TMP_DIR, exist_ok=True)

    print(f"Exporting {MODEL_NAME} to ONNX...")
    subprocess.run(
        [
            sys.executable, "-m", "optimum.exporters.onnx",
            "--model", MODEL_NAME,
            "--task", "feature-extraction",
            TMP_DIR,
        ],
        check=True,
    )

    # Copy only essential files
    for fname in KEEP_FILES:
        src = os.path.join(TMP_DIR, fname)
        dst = os.path.join(MODEL_DIR, fname)
        if os.path.exists(src):
            shutil.copy2(src, dst)
            size_mb = os.path.getsize(dst) / (1024 * 1024)
            print(f"  Copied {fname} ({size_mb:.1f} MB)")
        else:
            print(f"  WARNING: {fname} not found in export output")

    # Clean up temp dir
    shutil.rmtree(TMP_DIR, ignore_errors=True)

    print("\nModel files in", MODEL_DIR)
    for f in sorted(os.listdir(MODEL_DIR)):
        size = os.path.getsize(os.path.join(MODEL_DIR, f))
        print(f"  {f}: {size / (1024*1024):.1f} MB")

    print("\nDone!")


if __name__ == "__main__":
    main()
