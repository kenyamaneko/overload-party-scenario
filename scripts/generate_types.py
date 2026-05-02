#!/usr/bin/env python3
"""data/models.yaml から packages/api-scenario/*_gen.go を生成する.

共通基盤 `overload-party-codegen-tools` の `CodegenRunner` を使う。scenario は
出力先が単一 (packages/api-scenario) で、各セクションの `target`/`pkg` キーは
歴史的に YAML 上の宣言として残っているが現状は無視している。

実行: python3 scripts/generate_types.py
"""

from __future__ import annotations

import sys
from pathlib import Path

from codegen_tools import CodegenRunner, GoStyle, GoTarget

REPO_ROOT = Path(__file__).resolve().parent.parent
MODELS_YAML = REPO_ROOT / "data" / "models.yaml"


def main() -> int:
    runner = CodegenRunner(
        models_yaml=MODELS_YAML,
        repo_root=REPO_ROOT,
        targets={
            "default": GoTarget(
                out_dir=REPO_ROOT / "packages" / "api-scenario",
                package="apiscenario",
                emit_tags=("json", "db"),
            ),
        },
        style=GoStyle(),
        default_target_key="default",
        single_target_field=None,
        multi_target_field=None,
        trailing_blank_line=True,
    )
    return runner.run()


if __name__ == "__main__":
    sys.exit(main())
