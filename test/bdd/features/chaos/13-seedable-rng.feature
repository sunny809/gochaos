@chaos @priority=P2 @rng=seed
Feature: Seedable RNG
  As a developer using gmock
  I want to use deterministic random number generation with seeds
  So that chaos behavior is reproducible across test runs

  Background:
    Given a clean mock server on a random port

  @chaos @priority=P2 @rng=seed
  Scenario: Seed produces deterministic fault behavior
    Given a seed 42
    Given a stub with 50% probability of error fault
    When I send 20 requests to "/test"
    Then at least 5 error faults were injected
    Then at most 15 error faults were injected
