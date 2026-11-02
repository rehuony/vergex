-- A user with no @username was bound under a bare "@", which says nothing about who it is.
UPDATE telegram.bot SET username = '' WHERE username = '@';
