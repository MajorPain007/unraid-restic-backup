<?php
$rbToken = isset($var['csrf_token']) ? $var['csrf_token'] : '';
$rbBase = '/plugins/restic.backup/assets';
?>
<link rel="stylesheet" href="<?php autov("$rbBase/app.css"); ?>">
<div id="rb-app" class="rb" data-csrf="<?=htmlspecialchars($rbToken, ENT_QUOTES)?>"
     data-api="/plugins/restic.backup/include/api.php">
  <div class="rb-boot">Loading&hellip;</div>
</div>
<script src="<?php autov("$rbBase/app.js"); ?>"></script>
