"""Tests for SQL agent validation."""
import pytest
from agent.sql_agent import validate_sql


def test_valid_select():
    ok, err = validate_sql("SELECT COUNT(*) FROM activities WHERE user_id = :user_id")
    assert ok, err


def test_reject_insert():
    ok, err = validate_sql("INSERT INTO activities (title) VALUES ('hack')")
    assert not ok
    assert "SELECT" in err


def test_reject_delete():
    ok, err = validate_sql("DELETE FROM activities")
    assert not ok


def test_reject_drop():
    ok, err = validate_sql("DROP TABLE activities")
    assert not ok


def test_reject_update():
    ok, err = validate_sql("UPDATE activities SET title = 'hack'")
    assert not ok


def test_reject_pg_catalog():
    ok, err = validate_sql("SELECT * FROM pg_catalog.pg_tables")
    assert not ok


def test_reject_information_schema():
    ok, err = validate_sql("SELECT * FROM information_schema.tables")
    assert not ok


def test_reject_multi_statement():
    ok, err = validate_sql("SELECT 1; DROP TABLE activities")
    assert not ok


def test_accept_join():
    ok, err = validate_sql(
        "SELECT a.title, g.name FROM activities a JOIN gear g ON a.gear_id = g.id WHERE a.user_id = :user_id"
    )
    assert ok, err


def test_accept_aggregation():
    ok, err = validate_sql(
        "SELECT sport_type, COUNT(*) FROM activities WHERE user_id = :user_id GROUP BY sport_type"
    )
    assert ok, err
