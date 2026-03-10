"""Tests for semantic query router."""
import pytest
from chat.router import route_query, RouteResult
from chat.profiles import DEFAULT_PROFILES


def test_keyword_match_geo():
    r = route_query("Wie viele Gipfel habe ich bestiegen?", DEFAULT_PROFILES["small"])
    assert "geo" in r.domains
    assert r.confidence >= 0.8


def test_keyword_match_health():
    r = route_query("Wie ist mein HRV-Trend?", DEFAULT_PROFILES["small"])
    assert "health" in r.domains


def test_keyword_match_activities():
    r = route_query("Wann war mein letzter Lauf?", DEFAULT_PROFILES["small"])
    assert "activities" in r.domains


def test_keyword_match_gear():
    r = route_query("Wie viele km haben meine Schuhe?", DEFAULT_PROFILES["small"])
    assert "gear" in r.domains


def test_keyword_match_training():
    r = route_query("Wie ist meine CTL?", DEFAULT_PROFILES["small"])
    assert "training" in r.domains


def test_keyword_match_segments():
    r = route_query("Was ist meine Bestzeit auf dem Segment?", DEFAULT_PROFILES["small"])
    assert "segments" in r.domains


def test_multi_domain():
    r = route_query("Wie beeinflusst mein Schlaf mein Lauftraining?", DEFAULT_PROFILES["small"])
    assert len(r.domains) <= 2


def test_fallback_on_ambiguous():
    r = route_query("Hallo, wie geht es mir?", DEFAULT_PROFILES["small"])
    assert len(r.domains) >= 1  # should have a fallback


def test_large_profile_skips_routing():
    r = route_query("Wie viele Gipfel?", DEFAULT_PROFILES["large"])
    assert len(r.domains) == 7  # all domains


def test_max_two_domains():
    r = route_query("Gipfel Segment Bestzeit Berg", DEFAULT_PROFILES["small"])
    assert len(r.domains) <= 2
