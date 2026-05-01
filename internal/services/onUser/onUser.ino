const int LED_PIN = 7;

void setup() {
  pinMode(LED_PIN, OUTPUT);

  for (int i = 0; i < 5; i++) {
    digitalWrite(LED_PIN, HIGH);
    delay(500);
    digitalWrite(LED_PIN, LOW);
    delay(500);
  }

  // optional: ensure LED is OFF
  digitalWrite(LED_PIN, LOW);
}

void loop() {
  // idle forever (do nothing)
}
