//go:build !test && !js

// refs manages the reference-audio corpus: provenance in internal/wavref/catalogue.json, the audio only under
// $BEATMO_WAV_REFS, never in the repository. usage() lists every subcommand.
package main

import (
	"errors"
	"fmt"
	"os"
)

func usage() {
	fmt.Fprint(os.Stderr, `refs: manage the Beatmo reference-audio corpus

  refs list       [-tier instrument|song]
  refs families   [-instrument <family-or-member>]
  refs verify     [-id <ref-id>]
  refs doctor
  refs fetch      [-id <ref-id>] [-instrument <inst>] [-tier <t>] [-only-missing] [-force]
  refs candidates -instrument <inst> [-n 3]
  refs choose     -instrument <inst> -id <ref-id>
  refs adopt      -wav <name-or-path> [-instrument <inst>] [-note <n>] [-id <ref-id>] [-start-sec N] [-max-sec N]
  refs adopt      -scan [-note <n>] [-start-sec N] [-max-sec N]
  refs compare    -instrument <inst> [-ref <ref-id>] [-dur <sec>] [-match rms|peak|none] [-sh]
  refs compare    -list
  refs match      [-wavs <glob>] [-top 3] [-metric timbre|full] [-listen [-resume] [-only <text>]] [-validate]
  refs diagnose   [-instrument <inst>] [-all] [-json]
  refs audition   [-instrument <family-base>] [-out <verdicts.jsonl>] [-resume] [-sh]
  refs audition   -notes [-instrument <family-base>] [-only n,p] [-reannotate] [-out <verdicts.jsonl>] [-sh]
  refs validate   [-out <verdicts.jsonl>] [-min-samples N] [-terms]
  refs session    [-families a,b] [-candidates <json>] [-out <sessions.jsonl>] [-plan] [-clone | -tuned]
  refs tune       -inst <id[,id]> | -picks [-gens N] [-seed S] [-add-harmonics N] [-wavs <glob>] [-score <candidate>]
  refs probe      [-set <name>] [-sh]
  refs organic    -instrument <inst> [-pitch <semis>] [-dur <sec>]
  refs calibrate-organic [-out <organic_range.json>]
  refs calibrate-thresholds [-verdicts <verdicts.jsonl>] [-out <thresholds.json>]
  refs check      [-only thresholds,organic,f0,f0-outliers,candidates] [-v]
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "list":
		err = cmdList(os.Args[2:])
	case "families":
		err = cmdFamilies(os.Args[2:])
	case "verify":
		err = cmdVerify(os.Args[2:])
	case "doctor":
		err = cmdDoctor(os.Args[2:])
	case "fetch":
		err = cmdFetch(os.Args[2:])
	case "candidates":
		err = cmdCandidates(os.Args[2:])
	case "choose":
		err = cmdChoose(os.Args[2:])
	case "adopt":
		err = cmdAdopt(os.Args[2:])
	case "compare":
		err = cmdCompare(os.Args[2:])
	case "match":
		err = cmdMatch(os.Args[2:])
	case "diagnose":
		err = cmdDiagnose(os.Args[2:])
	case "audition":
		err = cmdAudition(os.Args[2:])
	case "validate":
		err = cmdValidate(os.Args[2:])
	case "session":
		err = cmdSession(os.Args[2:])
	case "tune":
		err = cmdTune(os.Args[2:])
	case "probe":
		err = cmdProbe(os.Args[2:])
	case "organic":
		err = cmdOrganic(os.Args[2:])
	case "calibrate-organic":
		err = cmdCalibrateOrganic(os.Args[2:])
	case "calibrate-thresholds":
		err = cmdCalibrateThresholds(os.Args[2:])
	case "check":
		err = cmdCheck(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	switch {
	case errors.Is(err, errAuditionQuit):
		os.Exit(exitAuditionQuit)
	case errors.Is(err, errAuditionReplay):
		os.Exit(exitAuditionReplay)
	case err != nil:
		fmt.Fprintf(os.Stderr, "refs: %v\n", err)
		os.Exit(1)
	}
}
