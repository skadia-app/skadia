import importlib.util
import sys
import types
from pathlib import Path


def _install_sklearn_stubs() -> None:
    sklearn = types.ModuleType("sklearn")
    cluster = types.ModuleType("sklearn.cluster")
    preprocessing = types.ModuleType("sklearn.preprocessing")

    class KMeans:
        pass

    class StandardScaler:
        pass

    cluster.KMeans = KMeans
    preprocessing.StandardScaler = StandardScaler
    sys.modules.setdefault("sklearn", sklearn)
    sys.modules.setdefault("sklearn.cluster", cluster)
    sys.modules.setdefault("sklearn.preprocessing", preprocessing)


_install_sklearn_stubs()

_MODULE_PATH = Path(__file__).resolve().parents[1] / "analytics" / "clustering.py"
_SPEC = importlib.util.spec_from_file_location("clustering_under_test", _MODULE_PATH)
assert _SPEC is not None
_MODULE = importlib.util.module_from_spec(_SPEC)
assert _SPEC.loader is not None
_SPEC.loader.exec_module(_MODULE)
_auto_label = _MODULE._auto_label
_auto_label_key = _MODULE._auto_label_key


def test_auto_label_uses_requested_supported_locale():
    center = {
        "duration_h": 3.5,
        "hr_pct": 60.0,
        "elevation_m": 100.0,
        "power_w": 120.0,
    }

    assert _auto_label(center, locale="en-US") == "Endurance"
    assert _auto_label(center, locale="fr") == "Endurance"
    assert _auto_label(center, locale="es") == "Resistencia"
    assert _auto_label(center, locale="it") == "Resistenza"
    assert _auto_label_key(center) == "endurance"


def test_auto_label_falls_back_to_german_for_unsupported_locale():
    center = {
        "duration_h": 0.5,
        "hr_pct": 60.0,
        "elevation_m": 100.0,
        "power_w": 120.0,
    }

    assert _auto_label(center, locale="pt-BR") == "Erholung"
