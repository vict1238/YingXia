Candidate 1 was never delivered to the user. Its executable SHA-256 was
9739e43c6ba8a51151ac3e7feed383187888ed871f3130b0cbdfa4ba3d60cf3f.
It passed production startup, history clear/write failure, preservation,
restart, duplicate launch and child shutdown checks. Actual libmpv playback
rendered the synthetic WebM at 7.958 seconds/frame 191, but native position
events arrived faster than the browser-derived sampling guard, preventing
history progress saves. The candidate was archived unchanged under build/
failed-candidates. The fixed final release has its own hash and successful
evidence in the parent directory. These files are diagnostic history.
