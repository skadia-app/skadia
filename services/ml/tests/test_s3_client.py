import s3_client


def clear_s3_env(monkeypatch):
    for key in (
        "SKADIA_STORAGE_MODE",
        "SKADIA_S3_PROVIDER",
        "S3_ENDPOINT",
        "GARAGE_ENDPOINT",
        "S3_ACCESS_KEY",
        "GARAGE_ACCESS_KEY",
        "S3_SECRET_KEY",
        "GARAGE_SECRET_KEY",
        "S3_REGION",
        "S3_FORCE_PATH_STYLE",
    ):
        monkeypatch.delenv(key, raising=False)


def test_external_s3_does_not_default_to_local_garage(monkeypatch):
    clear_s3_env(monkeypatch)
    monkeypatch.setenv("SKADIA_STORAGE_MODE", "s3-external")
    monkeypatch.setenv("S3_ENDPOINT", "https://s3.example.invalid")
    monkeypatch.setenv("S3_ACCESS_KEY", "external-access")
    monkeypatch.setenv("S3_SECRET_KEY", "external-secret")
    monkeypatch.setenv("S3_REGION", "external-region")

    kwargs = s3_client.s3_client_kwargs()

    assert kwargs["endpoint_url"] == "https://s3.example.invalid"
    assert kwargs["aws_access_key_id"] == "external-access"
    assert kwargs["aws_secret_access_key"] == "external-secret"
    assert kwargs["region_name"] == "external-region"
    assert "config" not in kwargs


def test_external_s3_requires_explicit_region(monkeypatch):
    clear_s3_env(monkeypatch)
    monkeypatch.setenv("SKADIA_STORAGE_MODE", "s3-external")
    monkeypatch.setenv("S3_ENDPOINT", "https://s3.example.invalid")
    monkeypatch.setenv("S3_ACCESS_KEY", "external-access")
    monkeypatch.setenv("S3_SECRET_KEY", "external-secret")

    try:
        s3_client.s3_client_kwargs()
    except ValueError as exc:
        assert "S3_REGION" in str(exc)
    else:
        raise AssertionError("s3-external without S3_REGION was accepted")


def test_external_s3_requires_explicit_endpoint(monkeypatch):
    clear_s3_env(monkeypatch)
    monkeypatch.setenv("SKADIA_STORAGE_MODE", "s3-external")
    monkeypatch.setenv("S3_ACCESS_KEY", "external-access")
    monkeypatch.setenv("S3_SECRET_KEY", "external-secret")
    monkeypatch.setenv("S3_REGION", "external-region")

    try:
        s3_client.s3_client_kwargs()
    except ValueError as exc:
        assert "S3_ENDPOINT" in str(exc)
    else:
        raise AssertionError("s3-external without S3_ENDPOINT was accepted")


def test_local_garage_mode_defaults_endpoint_and_path_style(monkeypatch):
    clear_s3_env(monkeypatch)
    monkeypatch.setenv("SKADIA_STORAGE_MODE", "s3-local-garage")
    monkeypatch.setenv("S3_ACCESS_KEY", "garage-access")
    monkeypatch.setenv("S3_SECRET_KEY", "garage-secret")

    kwargs = s3_client.s3_client_kwargs()

    assert kwargs["endpoint_url"] == "http://garage:3900"
    assert kwargs["aws_access_key_id"] == "garage-access"
    assert kwargs["aws_secret_access_key"] == "garage-secret"
    assert kwargs["region_name"] == "us-east-1"
    assert kwargs["config"].s3 == {"addressing_style": "path"}


def test_legacy_local_garage_provider_defaults_endpoint_and_path_style(monkeypatch):
    clear_s3_env(monkeypatch)
    monkeypatch.setenv("SKADIA_S3_PROVIDER", "local-garage")
    monkeypatch.setenv("S3_ACCESS_KEY", "garage-access")
    monkeypatch.setenv("S3_SECRET_KEY", "garage-secret")

    kwargs = s3_client.s3_client_kwargs()

    assert kwargs["endpoint_url"] == "http://garage:3900"
    assert kwargs["region_name"] == "us-east-1"
    assert kwargs["config"].s3 == {"addressing_style": "path"}
