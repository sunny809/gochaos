@chaos @priority=P2 @delay=all
Feature: Delay Distributions
  As a developer using gmock
  I want to simulate various response delay patterns
  So that I can test how my application handles latency

  Background:
    Given a clean mock server on a random port

  @chaos @priority=P2 @delay=fixed
  Scenario: Fixed delay adds constant latency
    Given a stub with error fault
    Given a fixed delay of 500ms
    When I send 1 requests to "/test"
    Then 1 error faults were injected

  @chaos @priority=P2 @delay=random
  Scenario: Random delay adds variable latency within a range
    Given a stub with error fault
    Given a random delay between 100ms and 500ms
    When I send 1 requests to "/test"
    Then 1 error faults were injected

  @chaos @priority=P2 @delay=lognormal
  Scenario: Lognormal delay simulates realistic tail latency
    Given a stub with error fault
    Given a lognormal delay with P50=200ms and P95=1000ms
    When I send 1 requests to "/test"
    Then 1 error faults were injected

  @chaos @priority=P2 @delay=timeout
  Scenario: Timeout delay exceeds client deadline
    Given a stub with error fault
    Given a timeout delay of 5000ms
    When I send 1 requests to "/test"
    Then 1 error faults were injected

  @chaos @priority=P2 @delay=dribble
  Scenario: Dribble delay sends response in chunks
    Given a stub with error fault
    Given a dribble delay with 10 chunks over 2000ms
    When I send 1 requests to "/test"
    Then 1 error faults were injected
